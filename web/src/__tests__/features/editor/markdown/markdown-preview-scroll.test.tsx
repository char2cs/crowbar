import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { createElement, StrictMode } from 'react'
import { beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'

const { workspaceFileExists, openWorkspaceFile, openExternalUrl } = vi.hoisted(() => ({
  workspaceFileExists: vi.fn(),
  openWorkspaceFile: vi.fn(async () => {}),
  openExternalUrl: vi.fn(async (_url: string) => {}),
}))
vi.mock('@/features/file-system/controllers/platform', () => ({ workspaceFileExists }))
vi.mock('@/features/files/lib/file-tree-handlers', () => ({ openWorkspaceFile }))
vi.mock('@/lib/external-open', () => ({ openExternalUrl }))

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
import {
  windowPaneStore,
  resetWindowPaneStoreForTests,
} from '@/features/panes/stores/window-pane-store'
import { setActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'
import { toast } from '@/features/window/stores/toast-store'
import { __resetWorkspaceScopesForTest, recordWorkspaceScope } from '@/lib/workspace-scope'
import {
  loadProjects,
  pairRemoteHost,
  projectOn,
  resetHostWorld,
} from '@/__tests__/__fixtures__/remote-host'

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
})

// A link in a preview resolves against, and opens in, the workspace the previewed file
// belongs to: panes are window-level, so that is not necessarily the one in view.
describe('MarkdownPreview links', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    clearPreservedScroll()
    setActiveWorkspaceId('w-in-view')
  })

  it('checks and opens a relative link in the previewed file’s own workspace', async () => {
    const store = setupStore(['docs/readme.md'])
    windowPaneStore.setState((s) => {
      s.buffers = s.buffers.map((b) =>
        b.id === 'src-docs/readme.md'
          ? ({ ...b, content: '[the spec](./spec)', workspaceId: 'w-buffer' } as typeof b)
          : b.id === 'preview-docs/readme.md'
            ? ({ ...b, workspaceId: 'w-buffer' } as typeof b)
            : b,
      )
      return s
    })
    workspaceFileExists.mockImplementation(async (_wsId: string, path: string) =>
      path.endsWith('.md'),
    )

    renderPreview(store, 'preview-docs/readme.md')
    const anchor = (await screen.findByText('the spec')).closest('a')!
    // Under jsdom the static renderer leaves the url on data-slate-url; the handler reads href.
    anchor.setAttribute('href', anchor.getAttribute('data-slate-url')!)
    fireEvent.click(anchor)

    await waitFor(() =>
      expect(openWorkspaceFile).toHaveBeenCalledExactlyOnceWith('w-buffer', 'docs/spec.md', {
        preview: true,
      }),
    )
    expect(workspaceFileExists.mock.calls.map(([wsId]) => wsId)).toEqual(['w-buffer', 'w-buffer'])
  })

  // A README on a remote host that says "open http://localhost:3000" means that host's port.
  describe('to a loopback address', () => {
    beforeEach(() => {
      resetHostWorld()
      __resetWorkspaceScopesForTest()
      vi.spyOn(toast, 'info').mockReturnValue('')
      pairRemoteHost('h1', 'Build box')
      loadProjects(projectOn('p-remote', 'h1'), projectOn('p-local', 'local'))
      recordWorkspaceScope({ projectId: 'p-remote', repoId: 'r', wsId: 'w-remote' })
      recordWorkspaceScope({ projectId: 'p-local', repoId: 'r', wsId: 'w-local' })
    })

    async function clickLinkIn(wsId: string, url: string) {
      const store = setupStore(['README.md'])
      windowPaneStore.setState((s) => {
        s.buffers = s.buffers.map((b) =>
          b.id === 'src-README.md'
            ? ({ ...b, content: `[the app](${url})`, workspaceId: wsId } as typeof b)
            : ({ ...b, workspaceId: wsId } as typeof b),
        )
        return s
      })
      renderPreview(store, 'preview-README.md')
      const anchor = (await screen.findByText('the app')).closest('a')!
      anchor.setAttribute('href', anchor.getAttribute('data-slate-url')!)
      fireEvent.click(anchor)
    }

    it('from a remote workspace names its host and opens nothing here', async () => {
      await clickLinkIn('w-remote', 'http://localhost:3000/')

      await waitFor(() => expect(toast.info).toHaveBeenCalledOnce())
      expect(vi.mocked(toast.info).mock.calls[0][0]).toMatch(/^This link is on Build box, not/)
      expect(openExternalUrl).not.toHaveBeenCalled()
    })

    it('from a Local workspace opens here', async () => {
      await clickLinkIn('w-local', 'http://localhost:3000/')

      await waitFor(() => expect(openExternalUrl).toHaveBeenCalledWith('http://localhost:3000/'))
      expect(toast.info).not.toHaveBeenCalled()
    })

    it('is not why a public address from a remote workspace stays shut', async () => {
      await clickLinkIn('w-remote', 'https://example.com/docs')

      await waitFor(() => expect(openExternalUrl).toHaveBeenCalledWith('https://example.com/docs'))
      expect(toast.info).not.toHaveBeenCalled()
    })
  })
})
