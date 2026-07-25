import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { useEffect } from 'react'
import { useMarkdownViewStore } from '@/features/editor/markdown/plate/markdown-view-store'

// Records every MOUNT (not render) of the Plate surface, plus the props it was
// handed — the pane is responsible for remounting it per buffer (C1) and for
// wiring the preview/promote seam (M3).
const plateMounts: { bufferId: string; isPreview?: boolean; onPromote?: () => void }[] = []
vi.mock('@/features/editor/markdown/plate/markdown-editor-pane', () => ({
  MarkdownEditorPane: (props: {
    bufferId: string
    isPreview?: boolean
    onPromote?: () => void
  }) => {
    useEffect(() => {
      plateMounts.push(props)
      // eslint-disable-next-line react-hooks/exhaustive-deps -- mount only
    }, [])
    return <div data-testid="plate-surface">{props.bufferId}</div>
  },
}))
vi.mock('@/features/editor/components/editor-surface', () => ({
  EditorSurface: () => <div data-testid="monaco-surface" />,
}))

let currentBuffer: { id: string; type: string; path: string; name: string; fileMissing?: boolean }
vi.mock('@/features/workspace/stores/hooks/use-buffer-store', () => ({
  useBufferById: (bufferId: string) => ({ ...currentBuffer, id: bufferId }),
}))
vi.mock('@/features/workspace/stores/workspace-context', () => ({
  useWorkspaceStore: () => ({ editorManager: {}, armEditor: async () => {} }),
}))

import { EditorPane } from '@/features/panes/components/editor-pane'

const baseProps = {
  paneId: 'p1',
  isActiveSurface: true,
  isPreview: false,
  onPromote: () => {},
}

beforeEach(() => {
  plateMounts.length = 0
})

describe('EditorPane surface selection', () => {
  it('renders Plate for a markdown buffer in rich view', async () => {
    currentBuffer = { id: 'b1', type: 'editor', path: '/r/README.md', name: 'README.md' }
    useMarkdownViewStore.setState({ views: {} }) // default rich
    render(<EditorPane {...baseProps} bufferId="b1" />)
    // The Plate surface is lazy-loaded (Suspense), so it doesn't appear on the
    // synchronous first render pass — wait for the lazy chunk to resolve.
    expect(await screen.findByTestId('plate-surface')).toBeInTheDocument()
    expect(screen.queryByTestId('monaco-surface')).toBeNull()
  })

  it('renders Monaco for a markdown buffer in source view', () => {
    currentBuffer = { id: 'b2', type: 'editor', path: '/r/README.md', name: 'README.md' }
    useMarkdownViewStore.setState({ views: { b2: 'source' } })
    render(<EditorPane {...baseProps} bufferId="b2" />)
    expect(screen.getByTestId('monaco-surface')).toBeInTheDocument()
  })

  it('renders Monaco for a non-markdown buffer', () => {
    currentBuffer = { id: 'b3', type: 'editor', path: '/r/main.ts', name: 'main.ts' }
    useMarkdownViewStore.setState({ views: {} })
    render(<EditorPane {...baseProps} bufferId="b3" />)
    expect(screen.getByTestId('monaco-surface')).toBeInTheDocument()
    expect(screen.queryByTestId('plate-surface')).toBeNull()
  })

  // C1 (CRITICAL): the pane tree renders the active buffer with no key, so a
  // `.md` -> `.md` tab switch reaches EditorPane as a prop update. The Plate
  // surface memoizes its parsed document for the life of the mount, so unless
  // this pane keys it by buffer, file A's document stays live under file B's id
  // — and the next edit writes A's whole text into B.
  it('remounts the Plate surface when the buffer changes', async () => {
    currentBuffer = { id: 'b1', type: 'editor', path: '/r/one.md', name: 'one.md' }
    useMarkdownViewStore.setState({ views: {} })
    const { rerender } = render(<EditorPane {...baseProps} bufferId="b1" />)
    await screen.findByTestId('plate-surface')

    rerender(<EditorPane {...baseProps} bufferId="b2" />)
    await screen.findByText('b2')

    expect(plateMounts.map((m) => m.bufferId)).toEqual(['b1', 'b2'])
  })

  // M3: the rich branch dropped isPreview/onPromote, so an edited markdown
  // preview tab never promoted to permanent (Monaco promotes on first edit) and
  // stayed evictable.
  it('hands the preview/promote seam to the Plate surface', async () => {
    const onPromote = vi.fn()
    currentBuffer = { id: 'b1', type: 'editor', path: '/r/one.md', name: 'one.md' }
    useMarkdownViewStore.setState({ views: {} })
    render(<EditorPane {...baseProps} bufferId="b1" isPreview onPromote={onPromote} />)
    await screen.findByTestId('plate-surface')

    expect(plateMounts).toHaveLength(1)
    expect(plateMounts[0].isPreview).toBe(true)
    plateMounts[0].onPromote?.()
    expect(onPromote).toHaveBeenCalledTimes(1)
  })
})
