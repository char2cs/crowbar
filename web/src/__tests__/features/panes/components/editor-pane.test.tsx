import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { useEffect, type ReactNode } from 'react'

// Records every MOUNT (not render) of the Plate surface, plus the props it was
// handed — the pane remounts it per buffer (C1) and wires preview/promote (M3).
const plateMounts: { bufferId: string; isPreview?: boolean; onPromote?: () => void }[] = []
vi.mock('@/features/editor/markdown/plate/markdown-editor-pane', () => ({
  MarkdownEditorPane: (props: {
    bufferId: string
    isPreview?: boolean
    onPromote?: () => void
  }) => {
    useEffect(() => {
      plateMounts.push(props)
      // eslint-disable-next-line react-hooks/exhaustive-deps -- records the mount only
    }, [])
    return <div data-testid="plate-surface">{props.bufferId}</div>
  },
}))

import { EditorPane } from '@/features/panes/components/editor-pane'
import { getEditorHost } from '@/features/editor/host/editor-host'
import { fileUri } from '@/features/editor/lib/editor-uri'
import { useMarkdownViewStore } from '@/features/editor/markdown/plate/markdown-view-store'
import {
  resetWindowPaneStoreForTests,
  windowPaneStore,
} from '@/features/panes/stores/window-pane-store'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'

// Real host and real Monaco: the pane loads the editor on first mount. A
// ceiling, not a wait — the monaco chunk takes seconds on a busy CI box.
const MONACO_LOAD_MS = 120_000

function open(path: string, workspaceId = 'ws-buffer'): string {
  return windowPaneStore
    .getState()
    .bufferActions.openContent({ type: 'editor', path, name: path, content: 'A', workspaceId })
}

function pane(bufferId: string, extra: { isPreview?: boolean; onPromote?: () => void } = {}) {
  const paneId = windowPaneStore.getState().activePaneId
  return (
    <EditorPane
      paneId={paneId}
      bufferId={bufferId}
      isActiveSurface
      isPreview={extra.isPreview ?? false}
      onPromote={extra.onPromote ?? (() => {})}
    />
  )
}

/** The pane tree renders under some workspace's ambient store. */
function inAmbient(node: ReactNode, ambientWsId = 'ws-ambient') {
  return (
    <WorkspaceStoreContext.Provider value={createWorkspaceStore(ambientWsId)}>
      {node}
    </WorkspaceStoreContext.Provider>
  )
}

function monacoMounted(): boolean {
  const paneId = windowPaneStore.getState().activePaneId
  return getEditorHost()?.manager.getEditor(paneId) !== undefined
}

beforeEach(() => {
  plateMounts.length = 0
  resetWindowPaneStoreForTests()
  useMarkdownViewStore.setState({ views: {} })
})

describe('EditorPane', () => {
  it(
    'renders Monaco for a buffer whose workspace has no store yet',
    async () => {
      const id = open('src/x.ts')
      render(inAmbient(pane(id)))

      await waitFor(() => expect(monacoMounted()).toBe(true), { timeout: MONACO_LOAD_MS })
      // The mount shows the active buffer in the same effect.
      const model = getEditorHost()?.models.get(fileUri('ws-buffer', 'src/x.ts'))
      expect(model?.getValue()).toBe('A')
    },
    MONACO_LOAD_MS,
  )

  it('routes rich markdown to Plate', async () => {
    const id = open('README.md')
    render(inAmbient(pane(id)))

    expect(await screen.findByTestId('plate-surface')).toBeInTheDocument()
    expect(monacoMounted()).toBe(false)
  })

  it(
    'routes source-view markdown to Monaco',
    async () => {
      const id = open('README.md')
      useMarkdownViewStore.setState({ views: { [id]: 'source' } })
      render(inAmbient(pane(id)))

      await waitFor(() => expect(monacoMounted()).toBe(true), { timeout: MONACO_LOAD_MS })
      expect(screen.queryByTestId('plate-surface')).toBeNull()
    },
    MONACO_LOAD_MS,
  )

  it('shows "File not found" for a missing buffer', () => {
    const id = open('src/gone.ts')
    windowPaneStore.setState((s) => ({
      buffers: s.buffers.map((b) => (b.id === id ? { ...b, fileMissing: true } : b)),
    }))
    render(inAmbient(pane(id)))

    expect(screen.getByText('File not found')).toBeInTheDocument()
    expect(monacoMounted()).toBe(false)
  })

  // C1: the pane tree renders the active buffer without a key, so a `.md` ->
  // `.md` tab switch is a prop update. Plate parses its document once per
  // mount; unkeyed, file A's document would stay live under B's id.
  it('remounts the Plate surface when the buffer changes', async () => {
    const one = open('one.md')
    const two = open('two.md')
    const { rerender } = render(inAmbient(pane(one)))
    await screen.findByText(one)

    rerender(inAmbient(pane(two)))
    await screen.findByText(two)

    expect(plateMounts.map((m) => m.bufferId)).toEqual([one, two])
  })

  // M3: an edited markdown preview tab must promote to permanent, as Monaco's does.
  it('hands the preview/promote seam to the Plate surface', async () => {
    const onPromote = vi.fn()
    const id = open('one.md')
    render(inAmbient(pane(id, { isPreview: true, onPromote })))
    await screen.findByTestId('plate-surface')

    expect(plateMounts).toHaveLength(1)
    expect(plateMounts[0].isPreview).toBe(true)
    plateMounts[0].onPromote?.()
    expect(onPromote).toHaveBeenCalledTimes(1)
  })
})
