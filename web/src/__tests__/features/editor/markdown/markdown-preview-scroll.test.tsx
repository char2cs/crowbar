import { fireEvent, render, waitFor } from '@testing-library/react'
import { createElement, StrictMode } from 'react'
import { beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'

const { openDocument, exists } = vi.hoisted(() => ({
  openDocument: vi.fn(async () => null),
  exists: vi.fn(async (_ws: string, _path: string) => true),
}))
vi.mock('@/features/editor/documents/open-document', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/features/editor/documents/open-document')>()),
  openDocument,
}))
vi.mock('@/features/file-system/controllers/platform', () => ({ exists }))

import { MarkdownPreview } from '@/features/editor/markdown/markdown-preview'
import { workspaceRef, type DocumentRef } from '@/lib/api/documents'
import { setActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'
import { clearPreservedScroll } from '@/features/editor/hooks/use-preserved-scroll'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import type {
  EditorContent,
  MarkdownPreviewContent,
  PaneContent,
} from '@/features/panes/types/pane-content'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'
import {
  windowPaneStore,
  resetWindowPaneStoreForTests,
} from '@/features/panes/stores/window-pane-store'

const LONG_MARKDOWN = Array.from({ length: 200 }, (_, i) => `# Heading ${i}\n\nBody ${i}\n`).join(
  '\n',
)

function makeSource(path: string, content: string): EditorContent {
  return {
    id: `src-${path}`,
    type: 'editor',
    authority: 'workspace',
    path,
    name: path.split('/').pop() ?? path,
    content,
    savedContent: content,
    isDirty: false,
    isVirtual: false,
    isPinned: false,
    isPreview: false,
    tokens: [],
    workspaceId: 'w1',
  }
}

function makePreview(
  path: string,
  source: DocumentRef = workspaceRef('w1', path),
): MarkdownPreviewContent {
  return {
    id: `preview-${path}`,
    type: 'markdownPreview',
    path: `${path} (preview)`,
    name: `Preview: ${path.split('/').pop()}`,
    content: '',
    source,
    isPinned: false,
    isPreview: false,
    workspaceId: source.authority === 'workspace' ? source.workspaceId : '',
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
  // The first Plate render pays its module and plugin setup; keep it out of
  // the timed cases.
  beforeAll(() => {
    const store = setupStore(['/repo/WARMUP.md'])
    renderPreview(store, 'preview-/repo/WARMUP.md').unmount()
  }, 30_000)

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

  it('renders its own workspace’s source when another workspace has the same path', () => {
    const store = setupStore(['/repo/README.md'])
    const foreign = { ...makeSource('/repo/README.md', '# Foreign heading'), id: 'src-w2' }
    windowPaneStore.setState((s) => {
      s.buffers = [{ ...foreign, workspaceId: 'w2' }, ...s.buffers]
      return s
    })

    const view = renderPreview(store, 'preview-/repo/README.md')
    expect(view.container.innerHTML).toContain('Heading 0')
    expect(view.container.innerHTML).not.toContain('Foreign heading')
  })
})

describe('MarkdownPreview links', () => {
  function previewOf(source: EditorContent, ref: DocumentRef) {
    const preview = makePreview(source.path ?? '', ref)
    resetWindowPaneStoreForTests()
    windowPaneStore.setState((s) => {
      s.buffers = [source, preview]
      s.panes[ROOT_PANE_ID] = {
        ...s.panes[ROOT_PANE_ID],
        editorTabIds: [source.id, preview.id],
        activeEditorTabId: preview.id,
      }
      return s
    })
    const view = renderPreview(createWorkspaceStore('w1'), preview.id)
    fireEvent.click(view.container.querySelector('a') as HTMLAnchorElement)
  }

  beforeEach(() => {
    openDocument.mockClear()
    exists.mockClear()
  })

  it('opens a relative link beside its source, in the source’s workspace', async () => {
    setActiveWorkspaceId('w-other')
    previewOf(
      makeSource('docs/README.md', '[spec](./spec.md)\n'),
      workspaceRef('w1', 'docs/README.md'),
    )

    await waitFor(() =>
      expect(openDocument).toHaveBeenCalledWith(workspaceRef('w1', 'docs/spec.md'), {
        preview: true,
      }),
    )
    expect(exists).toHaveBeenCalledWith('w1', 'docs/spec.md')
    setActiveWorkspaceId(null)
  })

  it('opens a host document’s link as a host document', async () => {
    const source = {
      ...makeSource('/tmp/notes/a.md', '[b](../other/b.md)\n'),
      authority: 'host' as const,
      workspaceId: '',
    }
    previewOf(source, { authority: 'host', path: '/tmp/notes/a.md' })

    await waitFor(() =>
      expect(openDocument).toHaveBeenCalledWith(
        { authority: 'host', path: '/tmp/other/b.md' },
        { preview: true },
      ),
    )
  })
})
