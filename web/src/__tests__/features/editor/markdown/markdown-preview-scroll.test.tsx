import { fireEvent, render } from '@testing-library/react'
import { createElement, StrictMode } from 'react'
import { beforeEach, describe, expect, it } from 'vitest'
import { MarkdownPreview } from '@/features/editor/markdown/markdown-preview'
import { clearPreservedScroll } from '@/features/editor/hooks/use-preserved-scroll'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import type {
  EditorContent,
  MarkdownPreviewContent,
  PaneContent,
} from '@/features/panes/types/pane-content'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'
import { windowPaneStore, resetWindowPaneStoreForTests } from '@/features/panes/stores/window-pane-store'

const LONG_MARKDOWN = Array.from({ length: 200 }, (_, i) => `# Heading ${i}\n\nBody ${i}\n`).join(
  '\n',
)

function makeSource(path: string, content: string): EditorContent {
  return {
    id: `src-${path}`,
    type: 'editor',
    path,
    name: path.split('/').pop() ?? path,
    content,
    savedContent: content,
    isDirty: false,
    isVirtual: false,
    isPinned: false,
    isPreview: false,
    isActive: false,
    tokens: [],
    workspaceId: 'w1',
  }
}

function makePreview(path: string): MarkdownPreviewContent {
  return {
    id: `preview-${path}`,
    type: 'markdownPreview',
    path: `${path} (preview)`,
    name: `Preview: ${path.split('/').pop()}`,
    content: '',
    sourceFilePath: path,
    isPinned: false,
    isPreview: false,
    isActive: false,
    workspaceId: 'w1',
  }
}

/**
 * Store holding a markdown source plus its preview buffer, with the preview as
 * the pane's active tab — the state PaneContainer renders MarkdownPreview in.
 * Task 26: panes/buffers are window-level now — seed `windowPaneStore`, not
 * the per-workspace store `createWorkspaceStore` still returns (kept only
 * because MarkdownPreview renders inside a WorkspaceStoreContext.Provider).
 */
function setupStore(paths: string[]) {
  const store = createWorkspaceStore('w1')
  const buffers: PaneContent[] = paths.flatMap((path) => [
    makeSource(path, LONG_MARKDOWN),
    makePreview(path),
  ])
  resetWindowPaneStoreForTests()
  windowPaneStore.setState((s) => {
    s.buffers = buffers
    s.panes[ROOT_PANE_ID] = {
      ...s.panes[ROOT_PANE_ID],
      editorTabIds: buffers.map((b) => b.id),
      activeEditorTabId: `preview-${paths[0]}`,
    }
    return s
  })
  return store
}

// Rendered under StrictMode, like the real app root: its double-invoked effects
// are what broke the first cut of the retained scroll.
function renderPreview(store: ReturnType<typeof createWorkspaceStore>, bufferId: string) {
  return render(
    createElement(
      StrictMode,
      null,
      createElement(
        WorkspaceStoreContext.Provider,
        { value: store },
        createElement(MarkdownPreview, { bufferId }),
      ),
    ),
  )
}

const scrollerOf = (view: ReturnType<typeof render>) =>
  view.container.querySelector('.markdown-preview') as HTMLElement

describe('MarkdownPreview scroll retention', () => {
  beforeEach(() => {
    clearPreservedScroll()
  })

  // Regression: switching the pane to another tab unmounts the preview (only the
  // active buffer is rendered), so its scroll offset used to reset to the top on
  // the way back.
  it('restores the scroll offset when the tab is switched away and back', () => {
    const store = setupStore(['/repo/README.md'])

    const first = renderPreview(store, 'preview-/repo/README.md')
    const scroller = scrollerOf(first)
    scroller.scrollTop = 640
    fireEvent.scroll(scroller)

    // Tab switch: PaneContainer stops rendering the preview entirely.
    first.unmount()

    const second = renderPreview(store, 'preview-/repo/README.md')
    expect(scrollerOf(second).scrollTop).toBe(640)
  })

  it('keeps each preview buffer at its own offset', () => {
    const store = setupStore(['/repo/README.md', '/repo/CHANGELOG.md'])

    const first = renderPreview(store, 'preview-/repo/README.md')
    const scroller = scrollerOf(first)
    scroller.scrollTop = 640
    fireEvent.scroll(scroller)
    first.unmount()

    const other = renderPreview(store, 'preview-/repo/CHANGELOG.md')
    expect(scrollerOf(other).scrollTop).toBe(0)
  })

  it('renders the buffer it was handed, not the active pane buffer', () => {
    const store = setupStore(['/repo/README.md', '/repo/CHANGELOG.md'])
    windowPaneStore.setState((s) => {
      s.buffers = s.buffers.map((b) =>
        b.id === 'src-/repo/CHANGELOG.md'
          ? ({ ...b, content: '# Changelog only heading' } as typeof b)
          : b,
      )
      return s
    })

    // The pane's active tab is the README preview; this instance is asked for
    // the CHANGELOG preview (the split-pane case).
    const view = renderPreview(store, 'preview-/repo/CHANGELOG.md')
    expect(view.container.innerHTML).toContain('Changelog only heading')
  })
})
