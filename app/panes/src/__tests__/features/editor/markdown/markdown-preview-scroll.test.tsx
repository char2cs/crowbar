import { fireEvent, render, waitFor } from '@testing-library/react'
import { createElement, StrictMode } from 'react'
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'

const { openDocument, exists, openExternalUrl } = vi.hoisted(() => ({
  openDocument: vi.fn(async () => null),
  exists: vi.fn(async (_ws: string, _path: string) => true),
  openExternalUrl: vi.fn(async (_url: string) => {}),
}))
vi.mock('@/features/editor/documents/open-document', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/features/editor/documents/open-document')>()),
  openDocument,
}))
vi.mock('@/features/file-system/controllers/platform', () => ({ exists }))
vi.mock('@/lib/external-open', () => ({ openExternalUrl }))

import { MONACO_LOAD_MS } from '@/__tests__/__fixtures__/documents'
import { replaceDocumentText } from '@/features/editor/documents/document-text'
import { getEditorHost, loadEditorHost } from '@/features/editor/host/editor-host'
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
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { resetWindowPaneStoreForTests } from '@/__tests__/__fixtures__/window-pane-store'
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

/** The open source document's tab; `doc`'s text is `text`. */
function makeSource(doc: DocumentRef, text: string): EditorContent {
  getEditorHost()!.documents.open(doc, 'markdown', text)
  return {
    id: `src-${doc.path}`,
    type: 'editor',
    authority: doc.authority,
    path: doc.path,
    name: doc.path.split('/').pop() ?? doc.path,
    isDirty: false,
    isVirtual: false,
    isPinned: false,
    isPreview: false,
    workspaceId: doc.authority === 'workspace' ? doc.workspaceId : '',
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

/** A markdown source plus its preview buffer, the preview active in the pane: the state
 *  PaneContainer renders MarkdownPreview in. The workspace store only provides the context. */
function setupStore(paths: string[]) {
  const store = createWorkspaceStore('w1')
  resetWindowPaneStoreForTests()
  const buffers: PaneContent[] = paths.flatMap((path) => [
    makeSource(workspaceRef('w1', path), LONG_MARKDOWN),
    makePreview(path),
  ])
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

beforeAll(() => loadEditorHost(), MONACO_LOAD_MS)

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

  // Switching tabs unmounts the preview (only the active buffer renders); the scroll offset must
  // survive the way back.
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
    replaceDocumentText(workspaceRef('w1', '/repo/CHANGELOG.md'), '# Changelog only heading')

    // The pane's active tab is the README preview; this instance is asked for
    // the CHANGELOG preview (the split-pane case).
    const view = renderPreview(store, 'preview-/repo/CHANGELOG.md')
    expect(view.container.innerHTML).toContain('Changelog only heading')
  })

  it('renders its own workspace’s source when another workspace has the same path', () => {
    const store = setupStore(['/repo/README.md'])
    const foreign = makeSource(workspaceRef('w2', '/repo/README.md'), '# Foreign heading')
    windowPaneStore.setState((s) => {
      s.buffers = [{ ...foreign, id: 'src-w2' }, ...s.buffers]
      return s
    })

    const view = renderPreview(store, 'preview-/repo/README.md')
    expect(view.container.innerHTML).toContain('Heading 0')
    expect(view.container.innerHTML).not.toContain('Foreign heading')
  })
})

describe('MarkdownPreview links', () => {
  function previewOf(ref: DocumentRef, text: string) {
    resetWindowPaneStoreForTests()
    const source = makeSource(ref, text)
    const preview = makePreview(ref.path, ref)
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
    previewOf(workspaceRef('w1', 'docs/README.md'), '[spec](./spec.md)\n')

    await waitFor(() =>
      expect(openDocument).toHaveBeenCalledWith(workspaceRef('w1', 'docs/spec.md'), {
        preview: true,
      }),
    )
    expect(exists).toHaveBeenCalledWith('w1', 'docs/spec.md')
    setActiveWorkspaceId(null)
  })

  it('opens a host document’s link as a host document', async () => {
    previewOf({ authority: 'host', path: '/tmp/notes/a.md' }, '[b](../other/b.md)\n')

    await waitFor(() =>
      expect(openDocument).toHaveBeenCalledWith(
        { authority: 'host', path: '/tmp/other/b.md' },
        { preview: true },
      ),
    )
  })
})

describe('MarkdownPreview text', () => {
  // A file is a document: a hard-wrapped paragraph reads as one, per CommonMark.
  it('joins a hard-wrapped paragraph instead of keeping its line breaks', () => {
    const store = setupStore(['/repo/WRAPPED.md'])
    replaceDocumentText(workspaceRef('w1', '/repo/WRAPPED.md'), 'one line\nwrapped here')
    const view = renderPreview(store, 'preview-/repo/WRAPPED.md')
    const prose = view.container.querySelector('.agent-prose') as HTMLElement
    expect(prose.textContent).toContain('wrapped here')
    expect(getComputedStyle(prose).whiteSpace).not.toBe('pre-wrap')
  })
})

// A README on a remote host that says "open http://localhost:3000" means that host's port.
describe('MarkdownPreview links to a loopback address', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    resetHostWorld()
    __resetWorkspaceScopesForTest()
    vi.spyOn(toast, 'info').mockReturnValue('')
    pairRemoteHost('h1', 'Build box')
    loadProjects(projectOn('p-remote', 'h1'), projectOn('p-local', 'local'))
    recordWorkspaceScope({ projectId: 'p-remote', repoId: 'r', wsId: 'w-remote' })
    recordWorkspaceScope({ projectId: 'p-local', repoId: 'r', wsId: 'w-local' })
  })
  afterEach(() => {
    vi.restoreAllMocks()
    resetHostWorld()
  })

  function clickLinkIn(wsId: string, url: string) {
    resetWindowPaneStoreForTests()
    const ref = workspaceRef(wsId, 'README.md')
    const source = makeSource(ref, `[the app](${url})\n`)
    const preview = makePreview(ref.path, ref)
    windowPaneStore.setState((s) => {
      s.buffers = [source, preview]
      s.panes[ROOT_PANE_ID] = {
        ...s.panes[ROOT_PANE_ID],
        editorTabIds: [source.id, preview.id],
        activeEditorTabId: preview.id,
      }
      return s
    })
    const view = renderPreview(createWorkspaceStore(wsId), preview.id)
    fireEvent.click(view.container.querySelector('a') as HTMLAnchorElement)
  }

  it('from a remote workspace names its host and opens nothing here', async () => {
    clickLinkIn('w-remote', 'http://localhost:3000/')

    await waitFor(() => expect(toast.info).toHaveBeenCalledOnce())
    expect(vi.mocked(toast.info).mock.calls[0][0]).toMatch(/^This link is on Build box, not/)
    expect(openExternalUrl).not.toHaveBeenCalled()
  })

  it('from a Local workspace opens here', async () => {
    clickLinkIn('w-local', 'http://localhost:3000/')

    await waitFor(() => expect(openExternalUrl).toHaveBeenCalledWith('http://localhost:3000/'))
    expect(toast.info).not.toHaveBeenCalled()
  })

  it('is not why a public address from a remote workspace stays shut', async () => {
    clickLinkIn('w-remote', 'https://example.com/docs')

    await waitFor(() => expect(openExternalUrl).toHaveBeenCalledWith('https://example.com/docs'))
    expect(toast.info).not.toHaveBeenCalled()
  })
})
