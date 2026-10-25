import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useStore } from 'zustand'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import type { BranchReviewContent } from '@/features/panes/types/pane-content'
import type { ReviewThread } from '@/features/workspace/stores/slices/branch-review-slice'
import type { LineRequest } from '@/features/git/components/review-diff-tab'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import {
  destroyWorkspaceStore,
  getOrCreateWorkspaceStore,
} from '@/features/workspace/stores/workspace-store-registry'
import type { GitDiff } from '@/features/git/types/git-types'
import {
  __resetWorkspaceScopesForTest,
  recordWorkspaceScope,
  setWorkspaceScope,
} from '@/lib/workspace-scope'
import type { ReviewState } from '@/features/git/api/review-api'
import { useSidebarStore } from '@/lib/store/sidebar'
import {
  loadProjects,
  pairRemoteHost,
  projectOn,
  resetHostWorld,
} from '@/__tests__/__fixtures__/remote-host'

const mocks = vi.hoisted(() => ({
  getReview: vi.fn<() => Promise<ReviewState>>(),
}))
vi.mock('@/features/git/api/review-api', () => ({
  getReview: mocks.getReview,
}))

// The real ReviewDiffTab is lazy and drives a web-component diff surface that
// jsdom cannot host; what is under test here is the load()/owning-chat-id
// wiring above it, not the diff renderer.
vi.mock('@/features/git/components/review-diff-tab', () => ({
  ReviewDiffTab: ({ lineRequest }: { lineRequest?: LineRequest }) => (
    <div data-testid="diff-tab" data-line={lineRequest ? lineRequest.lineNumber : ''} />
  ),
}))

// The Conversation view has its own test file; here it is only the thing the
// pane switches to and the source of a "Jump to line" request.
const JUMP_THREAD: ReviewThread = {
  id: 't1',
  filePath: 'src/a.ts',
  lineNumber: 12,
  startLine: 12,
  endLine: 12,
  side: 'new',
  messages: [],
  isResolved: false,
}
vi.mock('@/features/git/components/review-conversation-view', () => ({
  ReviewConversationView: ({ onJumpToLine }: { onJumpToLine: (t: ReviewThread) => void }) => (
    <button type="button" onClick={() => onJumpToLine(JUMP_THREAD)}>
      jump
    </button>
  ),
}))

import { BranchReviewPane } from '@/features/git/components/branch-review-pane'

function review(overrides: Partial<ReviewState> = {}): ReviewState {
  return {
    description: '',
    mergeStrategy: 'merge',
    diff: { files: [] } as unknown as ReviewState['diff'],
    threads: [],
    conversations: [],
    ...overrides,
  }
}

function renderPane(wsId: string, store: ReturnType<typeof createWorkspaceStore>) {
  return render(
    <WorkspaceStoreContext.Provider value={store}>
      <BranchReviewPane bufferId="buf-1" wsId={wsId} />
    </WorkspaceStoreContext.Provider>,
  )
}

beforeEach(() => {
  vi.clearAllMocks()
  __resetWorkspaceScopesForTest()
  mocks.getReview.mockResolvedValue(review())
})

// Regression: the route records a workspace's scope with NO chat id; only the
// sidebar's own async chat-list fetch later attaches owningChatId. getReview
// resolves through reviewBaseForWorkspace, which throws without one — load()
// caught that throw and set a PERMANENT 'error' diffStatus, with nothing to
// ever retry it even once the id arrived. The pane must wait instead.
describe('BranchReviewPane — owning-chat-id race', () => {
  it('does not fetch the review before an owning chat id is recorded', async () => {
    setWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws-race' })
    const store = createWorkspaceStore('ws-race')

    renderPane('ws-race', store)
    await act(async () => {})

    expect(mocks.getReview).not.toHaveBeenCalled()
    expect(store.getState().branchReview.diffStatus).toBe('idle')
  })

  it('fetches the review once the owning chat id arrives after mount', async () => {
    setWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws-race' })
    const store = createWorkspaceStore('ws-race')
    mocks.getReview.mockResolvedValue(review({ description: 'hello world' }))

    renderPane('ws-race', store)
    await act(async () => {})
    expect(mocks.getReview).not.toHaveBeenCalled()

    act(() => {
      recordWorkspaceScope({
        projectId: 'p1',
        repoId: 'r1',
        wsId: 'ws-race',
        owningChatId: 'chat-1',
      })
    })

    await waitFor(() => expect(store.getState().branchReview.description).toBe('hello world'))
    expect(mocks.getReview).toHaveBeenCalledWith('ws-race')
    // The race, not a real failure — the error status must never have been set.
    expect(store.getState().branchReview.diffStatus).not.toBe('error')
  })

  it('still sets the error status on a genuine fetch failure once scope is ready', async () => {
    setWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws-ready', owningChatId: 'chat-1' })
    const store = createWorkspaceStore('ws-ready')
    mocks.getReview.mockRejectedValue(new Error('502'))

    renderPane('ws-ready', store)

    await waitFor(() => expect(store.getState().branchReview.diffStatus).toBe('error'))
  })

  it('fetches immediately when the scope already carries an owning chat id', async () => {
    setWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws-ready-2', owningChatId: 'chat-2' })
    const store = createWorkspaceStore('ws-ready-2')
    mocks.getReview.mockResolvedValue(review({ description: 'already scoped' }))

    renderPane('ws-ready-2', store)

    await waitFor(() => expect(store.getState().branchReview.description).toBe('already scoped'))
    expect(mocks.getReview).toHaveBeenCalledWith('ws-ready-2')
  })
})

// The failed fetch below would land on a permanent 'error' status, so a paired
// install must not ask before the project list says whose daemon holds the review.
describe('BranchReviewPane — host not yet known', () => {
  beforeEach(() => {
    resetHostWorld()
    pairRemoteHost('host-1')
  })
  afterEach(() => resetHostWorld())

  it('fetches once the project list names the host, never marking an error before then', async () => {
    setWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws-h', owningChatId: 'chat-h' })
    const store = createWorkspaceStore('ws-h')
    mocks.getReview.mockResolvedValue(review({ description: 'hello host' }))

    renderPane('ws-h', store)
    await act(async () => {})
    expect(mocks.getReview).not.toHaveBeenCalled()
    expect(store.getState().branchReview.diffStatus).toBe('idle')

    act(() => loadProjects(projectOn('p1', 'local')))

    await waitFor(() => expect(store.getState().branchReview.description).toBe('hello host'))
    expect(mocks.getReview).toHaveBeenCalledExactlyOnceWith('ws-h')
  })
})

function openReviewBuffer(wsId: string): string {
  return windowPaneStore
    .getState()
    .bufferActions.openContent({ type: 'branchReview', wsId, name: 'Branch Review' })
}

// Mirrors the registry: the persisted `view` on the buffer is what the pane renders.
function PersistedViewPane({ bufferId, wsId }: { bufferId: string; wsId: string }) {
  const view = useStore(
    windowPaneStore,
    (s) => (s.buffers.find((b) => b.id === bufferId) as BranchReviewContent | undefined)?.view,
  )
  return <BranchReviewPane bufferId={bufferId} view={view} wsId={wsId} />
}

function renderPersisted(wsId: string, bufferId: string) {
  setWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId, owningChatId: 'chat-1' })
  const store = createWorkspaceStore(wsId)
  const utils = render(
    <WorkspaceStoreContext.Provider value={store}>
      <PersistedViewPane bufferId={bufferId} wsId={wsId} />
    </WorkspaceStoreContext.Provider>,
  )
  return { ...utils, store }
}

function diffFile(path: string): GitDiff {
  return {
    file_path: path,
    is_new: false,
    is_deleted: false,
    is_renamed: false,
    additions: 1,
    deletions: 0,
    lines: [],
  }
}

describe('BranchReviewPane — Conversation / Files changed shell', () => {
  it('opens on Conversation and persists the tab the user picks on the buffer', async () => {
    const user = userEvent.setup()
    const id = openReviewBuffer('ws-tabs')
    renderPersisted('ws-tabs', id)

    expect(screen.getByRole('button', { name: 'jump' })).toBeDefined()
    expect(screen.queryByTestId('diff-tab')).toBeNull()

    await user.click(screen.getByRole('tab', { name: /Files changed/ }))

    expect(screen.getByTestId('diff-tab')).toBeDefined()
    const buf = windowPaneStore.getState().bufferActions.getBufferById(id)
    expect((buf as BranchReviewContent).view).toBe('files')
  })

  it('shows the changed-file count on the Files changed tab', async () => {
    getOrCreateWorkspaceStore('ws-count').setState({
      reviewFiles: [diffFile('a.ts'), diffFile('b.ts')],
    })
    const id = openReviewBuffer('ws-count')
    renderPersisted('ws-count', id)

    expect(await screen.findByRole('tab', { name: /Files changed\s*2/ })).toBeDefined()
    destroyWorkspaceStore('ws-count')
  })

  it('shows branch line totals from the files summary and no state chip', async () => {
    getOrCreateWorkspaceStore('ws-totals').setState({
      reviewFiles: [
        { ...diffFile('a.ts'), additions: 5, deletions: 2 },
        { ...diffFile('b.ts'), additions: 3, deletions: -1 },
      ],
    })
    const id = openReviewBuffer('ws-totals')
    renderPersisted('ws-totals', id)

    expect(await screen.findByText('+8')).toBeDefined()
    expect(screen.getByText('-2')).toBeDefined()
    expect(screen.getByText('2 files')).toBeDefined()
    expect(screen.queryByText('Open')).toBeNull()
    destroyWorkspaceStore('ws-totals')
  })

  it('renders no merge card; it lives in the sidebar Changes panel', async () => {
    const user = userEvent.setup()
    const id = openReviewBuffer('ws-merge-head')
    renderPersisted('ws-merge-head', id)

    expect(screen.queryByRole('region', { name: 'Merge' })).toBeNull()
    await user.click(screen.getByRole('tab', { name: /Files changed/ }))
    expect(screen.queryByRole('region', { name: 'Merge' })).toBeNull()
  })

  it('shows the owning chat title as the heading, and the thread count on Conversation', async () => {
    useSidebarStore.setState({
      repos: [
        {
          id: 'r1',
          name: 'repo',
          avatarLabel: 'R',
          avatarColor: '#000',
          workspaces: [
            {
              id: 'ws-title',
              branch: 'feat/x',
              age: '1h',
              owningChatId: 'chat-1',
              parentBranch: 'develop',
            },
          ],
          chats: [{ id: 'chat-1', repoId: 'r1', title: 'Stop UI leaking', order: 0 }],
        },
      ],
    })
    const id = openReviewBuffer('ws-title')
    const { store } = renderPersisted('ws-title', id)
    store.getState().upsertReviewThread({
      id: 't1',
      filePath: 'a.ts',
      lineNumber: 1,
      startLine: 1,
      endLine: 1,
      side: 'new',
      isResolved: false,
      messages: [],
    })

    expect(screen.getByRole('heading', { name: 'Stop UI leaking' })).toBeDefined()
    expect(screen.getByText('feat/x')).toBeDefined()
    expect(await screen.findByRole('tab', { name: /Conversation\s*1/ })).toBeDefined()
  })

  it('keeps the header and tab row mounted when switching tabs, and arrows move between them', async () => {
    const user = userEvent.setup()
    const id = openReviewBuffer('ws-stable')
    renderPersisted('ws-stable', id)

    const heading = screen.getByRole('heading')
    const tablist = screen.getByRole('tablist')
    await user.click(screen.getByRole('tab', { name: /Conversation/ }))
    await user.keyboard('{ArrowRight}')

    expect(screen.getByRole('tab', { name: /Files changed/ }).getAttribute('aria-selected')).toBe(
      'true',
    )
    expect(screen.getByRole('heading')).toBe(heading)
    expect(screen.getByRole('tablist')).toBe(tablist)
  })

  it('Jump to line switches to Files changed and hands the line to the diff surface', async () => {
    const user = userEvent.setup()
    const id = openReviewBuffer('ws-jump')
    windowPaneStore.getState().bufferActions.setBranchReviewView(id, 'conversation')
    renderPersisted('ws-jump', id)

    await user.click(screen.getByRole('button', { name: 'jump' }))

    expect(screen.getByTestId('diff-tab').getAttribute('data-line')).toBe('12')
    expect(
      (windowPaneStore.getState().bufferActions.getBufferById(id) as BranchReviewContent).view,
    ).toBe('files')
  })

  it('does not replay a jump the next time Files changed mounts', async () => {
    const user = userEvent.setup()
    const id = openReviewBuffer('ws-replay')
    windowPaneStore.getState().bufferActions.setBranchReviewView(id, 'conversation')
    renderPersisted('ws-replay', id)

    await user.click(screen.getByRole('button', { name: 'jump' }))
    await user.click(screen.getByRole('tab', { name: /Conversation/ }))
    await user.click(screen.getByRole('tab', { name: /Files changed/ }))

    expect(screen.getByTestId('diff-tab').getAttribute('data-line')).toBe('')
  })
})
