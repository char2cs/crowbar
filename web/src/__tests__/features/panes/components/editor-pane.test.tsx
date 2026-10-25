import { beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { useEffect, type ReactNode } from 'react'

// Records every MOUNT (not render) of the Plate surface: the pane remounts it per buffer (C1).
const plateMounts: { bufferId: string }[] = []
vi.mock('@/features/editor/markdown/plate/markdown-editor-pane', () => ({
  MarkdownEditorPane: (props: { bufferId: string }) => {
    useEffect(() => {
      plateMounts.push(props)
      // eslint-disable-next-line react-hooks/exhaustive-deps -- records the mount only
    }, [])
    return <div data-testid="plate-surface">{props.bufferId}</div>
  },
}))

import { MONACO_LOAD_MS, openTestDocument } from '@/__tests__/__fixtures__/documents'
import { EditorPane } from '@/features/panes/components/editor-pane'
import { getEditorHost, loadEditorHost } from '@/features/editor/host/editor-host'
import { documentText, replaceDocumentText } from '@/features/editor/documents/document-text'
import { useMarkdownViewStore } from '@/features/editor/markdown/plate/markdown-view-store'
import {
  resetWindowPaneStoreForTests,
  windowPaneStore,
} from '@/features/panes/stores/window-pane-store'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'
import { workspaceRef } from '@/lib/api/documents'
import { documentUri } from '@/features/editor/documents/document-ref'
import { putDrafts } from '@/features/editor/documents/draft-store'
import { setDocumentUnavailable } from '@/features/editor/documents/open-document'

const open = (path: string, opts: { isPreview?: boolean } = {}) =>
  openTestDocument(workspaceRef('ws-buffer', path), 'A', opts)

function pane(bufferId: string) {
  const paneId = windowPaneStore.getState().activePaneId
  return <EditorPane paneId={paneId} bufferId={bufferId} isActiveSurface />
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

beforeAll(() => loadEditorHost(), MONACO_LOAD_MS)

beforeEach(() => {
  plateMounts.length = 0
  resetWindowPaneStoreForTests()
  useMarkdownViewStore.setState({ views: {} })
})

describe('EditorPane', () => {
  it(
    'renders Monaco for a buffer whose workspace has no store yet',
    async () => {
      const id = await open('src/x.ts')
      render(inAmbient(pane(id)))

      await waitFor(() => expect(monacoMounted()).toBe(true), { timeout: MONACO_LOAD_MS })
      // The mount shows the active buffer in the same effect.
      const paneId = windowPaneStore.getState().activePaneId
      const shown = getEditorHost()?.manager.getEditor(paneId)?.getModel()
      expect(shown?.getValue()).toBe('A')
    },
    MONACO_LOAD_MS,
  )

  it('routes rich markdown to Plate', async () => {
    const id = await open('README.md')
    render(inAmbient(pane(id)))

    expect(await screen.findByTestId('plate-surface')).toBeInTheDocument()
    expect(monacoMounted()).toBe(false)
  })

  it(
    'routes source-view markdown to Monaco',
    async () => {
      const id = await open('README.md')
      useMarkdownViewStore.setState({ views: { [id]: 'source' } })
      render(inAmbient(pane(id)))

      await waitFor(() => expect(monacoMounted()).toBe(true), { timeout: MONACO_LOAD_MS })
      expect(screen.queryByTestId('plate-surface')).toBeNull()
    },
    MONACO_LOAD_MS,
  )

  it('says why a document could not be loaded instead of showing an editor', async () => {
    const id = await open('src/gone.ts')
    const unavailable = { title: 'Failed to open file', message: 'gone.ts' }
    windowPaneStore.setState((s) => ({
      buffers: s.buffers.map((b) => (b.id === id ? { ...b, unavailable } : b)),
    }))
    render(inAmbient(pane(id)))

    expect(screen.getByText('Failed to open file')).toBeInTheDocument()
    expect(screen.getByText('gone.ts')).toBeInTheDocument()
    expect(monacoMounted()).toBe(false)
  })

  it('a tab whose document failed to load reads it again on request', async () => {
    const doc = workspaceRef('ws-buffer', 'untitled:Untitled-7')
    const id = windowPaneStore
      .getState()
      .bufferActions.openContent({ type: 'editor', doc, name: 'Untitled-7', isVirtual: true })
    setDocumentUnavailable(doc, { title: 'Failed to open file', message: 'Untitled-7' })
    await putDrafts([{ uri: documentUri(doc), text: 'scratch', savedText: '', updatedAt: 0 }])
    render(inAmbient(pane(id)))

    fireEvent.click(screen.getByRole('button', { name: 'Try again' }))

    await waitFor(() => expect(documentText(doc)).toBe('scratch'))
    expect(screen.queryByText('Failed to open file')).not.toBeInTheDocument()
  })

  // C1: the pane tree renders the active buffer without a key, so a `.md` ->
  // `.md` tab switch is a prop update. Plate parses its document once per
  // mount; unkeyed, file A's document would stay live under B's id.
  it('remounts the Plate surface when the buffer changes', async () => {
    const one = await open('one.md')
    const two = await open('two.md')
    const { rerender } = render(inAmbient(pane(one)))
    await screen.findByText(one)

    rerender(inAmbient(pane(two)))
    await screen.findByText(two)

    expect(plateMounts.map((m) => m.bufferId)).toEqual([one, two])
  })

  // Plate parses its document once per mount, so it must not mount before the text exists.
  it('mounts Plate for rich markdown only once the document has opened', async () => {
    const doc = workspaceRef('ws-buffer', 'late.md')
    const id = windowPaneStore
      .getState()
      .bufferActions.openContent({ type: 'editor', doc, name: 'late.md' })
    render(inAmbient(pane(id)))
    expect(screen.queryByTestId('plate-surface')).toBeNull()

    await act(async () => void (await loadEditorHost()).documents.open(doc, 'markdown', '# late'))

    expect(await screen.findByTestId('plate-surface')).toBeInTheDocument()
    expect(documentText(doc)).toBe('# late')
  })

  // M3: an edited preview tab promotes to permanent, from either surface.
  it('promotes a preview tab once its document is edited', async () => {
    const id = await open('one.md', { isPreview: true })

    replaceDocumentText(workspaceRef('ws-buffer', 'one.md'), 'edited')

    expect(windowPaneStore.getState().buffers.find((b) => b.id === id)?.isPreview).toBe(false)
  })
})
