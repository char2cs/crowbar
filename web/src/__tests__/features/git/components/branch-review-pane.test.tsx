import { describe, it, expect, vi, beforeEach } from 'vitest'
import { act, render, waitFor } from '@testing-library/react'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import {
  __resetWorkspaceScopesForTest,
  recordWorkspaceScope,
  setWorkspaceScope,
} from '@/lib/workspace-scope'
import type { ReviewState } from '@/features/git/api/review-api'

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
  ReviewDiffTab: () => null,
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
      <BranchReviewPane wsId={wsId} />
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
