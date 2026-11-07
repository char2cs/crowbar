import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ReviewMergeBox } from '@/features/git/components/review-merge-box'
import type { GitStatus } from '@/features/git/types/git-types'
import {
  getOrCreateWorkspaceStore,
  setActiveWorkspaceId,
} from '@/features/workspace/stores/workspace-store-registry'
import type { ReviewThread } from '@/features/workspace/stores/slices/branch-review-slice'
import type { Repo, Workspace } from '@/core/entities/repo-types'
import { operationsState } from '@/core/entities/operations'
import { forgetEntities } from '@/core/entities/entity-store'
import { resetOperations, streamOperation } from '@/__tests__/__fixtures__/operations'
import { seedRepos } from '@/__tests__/__fixtures__/sidebar-entities'
import { clearReviewThreads, putReviewThread } from '@/__tests__/__fixtures__/review-threads'
import { seedChats } from '@/__tests__/__fixtures__/agent-chat'
import { ApiError } from '@/lib/errors/api-error'
import { onGitRefreshRequested } from '@/features/git/stores/git-refresh'

const mocks = vi.hoisted(() => ({
  setMergeStrategy: vi.fn(),
  mergeIntoParent: vi.fn(),
  navigate: vi.fn(),
  stopChat: vi.fn(),
}))
vi.mock('@/features/git/api/review-api', async (importOriginal) => ({
  ...(await importOriginal<object>()),
  setMergeStrategy: mocks.setMergeStrategy,
  mergeIntoParent: mocks.mergeIntoParent,
}))
vi.mock('@tanstack/react-router', () => ({ useNavigate: () => mocks.navigate }))
vi.mock('@/features/agent/api/agent-chat-commands', async (importOriginal) => ({
  ...(await importOriginal<object>()),
  stopChat: mocks.stopChat,
}))

const WS = 'ws-merge'
const PARENT_ROUTE = {
  to: '/ide/$projectId/$repoId/$wsId',
  params: { projectId: 'p1', repoId: 'r1', wsId: 'parent-ws' },
}

function openThread(id: string, isResolved = false): ReviewThread {
  return {
    id,
    filePath: 'a.ts',
    lineNumber: 1,
    startLine: 1,
    endLine: 1,
    side: 'new',
    messages: [{ id: `${id}-root`, author: 'me', isAgent: false, body: 'fix this', createdAt: '' }],
    isResolved,
  }
}

/** The fold's frame: the merged child leaves the sidebar. */
function removeTheChild() {
  forgetEntities('workspace', [WS])
}

const settledOp = () => Object.values(operationsState().ops)[0]?.state

function setup(opts: {
  ws?: Partial<Workspace>
  git?: Partial<GitStatus>
  threads?: ReviewThread[]
}) {
  const ws: Workspace = {
    id: WS,
    branch: 'feature',
    parentId: 'parent-ws',
    age: '1m',
    status: 'new',
    canMergeLocally: true,
    parentBranch: 'develop',
    ...opts.ws,
  }
  seedRepos([
    { id: 'r1', projectId: 'p1', defaultWorkspaceId: 'parent-ws', workspaces: [ws] } as Repo,
  ])
  const store = getOrCreateWorkspaceStore(WS)
  store.setState({
    gitStatus: { branch: 'feature', ahead: 0, behind: 0, files: [], ...opts.git },
    reviewFiles: null,
  })
  store.setState((s) => ({ branchReview: { ...s.branchReview, mergeStrategy: 'merge' } }))
  for (const thread of opts.threads ?? []) putReviewThread(WS, thread)
  return { store, ...render(<ReviewMergeBox wsId={WS} />) }
}

const mergeButton = () =>
  screen.getByRole('button', { name: 'Create merge commit' }) as HTMLButtonElement

/** Merges a child whose fold frame has not arrived yet. */
async function mergeBeforeTheFold() {
  mocks.mergeIntoParent.mockResolvedValue({ items: [] })
  const view = setup({})
  await userEvent.setup().click(mergeButton())
  return view
}

beforeEach(() => {
  vi.clearAllMocks()
  resetOperations()
  clearReviewThreads()
  setActiveWorkspaceId(WS)
  mocks.setMergeStrategy.mockResolvedValue('squash')
  mocks.mergeIntoParent.mockImplementation(async () => {
    removeTheChild()
    return { items: [] }
  })
})

describe('ReviewMergeBox', () => {
  it('merges as an operation only when resolveBranchAction says merge, then leaves for the parent', async () => {
    const user = userEvent.setup()
    setup({})
    expect(mergeButton().disabled).toBe(false)

    await user.click(mergeButton())

    const [op] = Object.values(operationsState().ops)
    expect(op).toMatchObject({
      kind: 'workspace.merge_into_parent',
      subject: { kind: 'workspace', id: WS },
    })
    expect(mocks.mergeIntoParent).toHaveBeenCalledWith(WS, 'merge', true, op.id)
    await waitFor(() => expect(mocks.navigate).toHaveBeenCalledWith(PARENT_ROUTE))
  })

  it('a branch with uncommitted changes can be committed right where merge asks for it', () => {
    setup({ git: { files: [{ path: 'a.ts', status: 'modified', staged: false }] } })

    expect(mergeButton().disabled).toBe(true)
    expect(screen.getByText('1 uncommitted change — commit them first.')).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Commit 1 change' })).toBeTruthy()
  })

  // Merging a branch with no changes merges nothing but would still delete the workspace.
  it('offers no merge while the branch has no changes against its parent', () => {
    const { store } = setup({})
    act(() => store.setState({ reviewFiles: [] }))

    expect(mergeButton().disabled).toBe(true)
    expect(screen.getByText('Nothing to merge yet — this branch has no changes.')).toBeTruthy()
  })

  it('keeps the workspace, stays put and refreshes it when delete-after-merge is unticked', async () => {
    const refreshed = vi.fn()
    onGitRefreshRequested(WS, refreshed)
    const user = userEvent.setup()
    setup({})
    await user.click(screen.getByText('Delete this branch and its chats after merging'))
    await user.click(mergeButton())

    expect(mocks.mergeIntoParent).toHaveBeenCalledWith(WS, 'merge', false, expect.any(String))
    expect(mocks.navigate).not.toHaveBeenCalled()
    await waitFor(() => expect(refreshed).toHaveBeenCalled())
  })

  it('a merge that kept the child (conflicts) does not redirect', async () => {
    await mergeBeforeTheFold()

    await waitFor(() => expect(settledOp()).toBe('succeeded'))
    expect(mocks.navigate).not.toHaveBeenCalled()
  })

  it('redirects when the fold frame arrives after the operation settled', async () => {
    await mergeBeforeTheFold()
    await waitFor(() => expect(settledOp()).toBe('succeeded'))

    act(() => removeTheChild())

    expect(mocks.navigate).toHaveBeenCalledTimes(1)
  })

  // The fold's frame lands before the terminal one and the route guard moves off
  // the vanished child at once: the user still lands on the parent, not home.
  it('lands on the parent when the child leaves while the merge still runs', async () => {
    let settle: (reply: { items: never[] }) => void = () => undefined
    mocks.mergeIntoParent.mockImplementation(() => new Promise((resolve) => (settle = resolve)))
    const user = userEvent.setup()
    setup({})
    await user.click(mergeButton())

    act(() => removeTheChild())
    act(() => setActiveWorkspaceId('fallback'))
    await act(async () => settle({ items: [] }))

    expect(mocks.navigate).toHaveBeenCalledTimes(1)
    expect(mocks.navigate).toHaveBeenCalledWith(PARENT_ROUTE)
  })

  it('does not redirect once the user has moved to another workspace', async () => {
    await mergeBeforeTheFold()
    await waitFor(() => expect(settledOp()).toBe('succeeded'))

    act(() => setActiveWorkspaceId('elsewhere'))
    act(() => removeTheChild())

    expect(mocks.navigate).not.toHaveBeenCalled()
  })

  it('stops waiting for the fold once the box is gone', async () => {
    const { unmount } = await mergeBeforeTheFold()
    await waitFor(() => expect(settledOp()).toBe('succeeded'))

    unmount()
    act(() => removeTheChild())

    expect(mocks.navigate).not.toHaveBeenCalled()
  })

  it('cannot merge while an operation on the workspace runs', () => {
    streamOperation({
      ...{ id: 'busy', kind: 'workspace.sync', subject: { kind: 'workspace', id: WS } },
      ...{ projectId: 'p1', label: 'Sync', state: 'running', items: [], dismissed: false },
      ...{ version: 1, createdAt: '', updatedAt: '' },
    })
    setup({})

    expect(screen.getByRole('button', { name: 'Merging…' })).toBeDisabled()
  })

  it('a failed merge does not redirect, and Merge is enabled again', async () => {
    mocks.mergeIntoParent.mockRejectedValue(new Error('conflict'))
    const user = userEvent.setup()
    setup({})
    await user.click(mergeButton())

    await waitFor(() => expect(settledOp()).toBe('failed'))
    expect(mocks.navigate).not.toHaveBeenCalled()
    expect(mergeButton().disabled).toBe(false)
  })

  it('says why a merge that would delete a working chat was refused, and can stop it and merge', async () => {
    mocks.stopChat.mockResolvedValue(undefined)
    mocks.mergeIntoParent.mockRejectedValueOnce(
      new ApiError('the chat is working: stop it first', 409, 'chat_busy'),
    )
    const user = userEvent.setup()
    setup({})
    seedChats([
      { id: 'chat-busy', workspaceId: WS, working: true },
      { id: 'chat-elsewhere', workspaceId: 'parent-ws', working: true },
    ])
    await user.click(mergeButton())

    expect(await screen.findByText('the chat is working: stop it first')).toBeTruthy()
    await user.click(screen.getByRole('button', { name: 'Stop and merge' }))

    await waitFor(() => expect(mocks.mergeIntoParent).toHaveBeenCalledTimes(2))
    expect(mocks.stopChat.mock.calls).toEqual([[WS, 'chat-busy']])
    expect(mocks.stopChat.mock.invocationCallOrder[0]).toBeLessThan(
      mocks.mergeIntoParent.mock.invocationCallOrder[1],
    )
    expect(mocks.mergeIntoParent.mock.calls[1].slice(0, 3)).toEqual([WS, 'merge', true])
  })

  it('is blocked by open threads, but resolved ones do not count', () => {
    setup({ threads: [openThread('a'), openThread('b'), openThread('c', true)] })
    expect(mergeButton().disabled).toBe(true)
    expect(screen.getByText('2 open threads must be resolved before merging.')).toBeDefined()
  })

  it.each([
    [
      'uncommitted changes',
      { git: { files: [{ path: 'x', status: 'M' }] as never } },
      '1 uncommitted change — commit them first.',
    ],
    [
      'a conflict',
      { ws: { status: 'pr-conflicts' as const } },
      'Conflicts with develop — resolve them first.',
    ],
    [
      'a protected parent',
      { ws: { canMergeLocally: false } },
      'develop is protected — open a pull request instead.',
    ],
    ['no parent', { ws: { parentBranch: undefined } }, 'This branch has no parent to merge into.'],
  ])('is blocked by %s and says why', (_name, opts, reason) => {
    setup(opts)
    expect(mergeButton().disabled).toBe(true)
    expect(screen.getByText(reason)).toBeDefined()
  })

  it('shows a merged branch as merged with nothing left to press', () => {
    setup({ ws: { status: 'pr-merged' } })
    expect(screen.getByText('Merged into develop')).toBeDefined()
    expect(screen.queryByRole('button', { name: /merge/i })).toBeNull()
  })

  it('saves a new strategy optimistically and reverts it when saving fails', async () => {
    const user = userEvent.setup()
    mocks.setMergeStrategy.mockRejectedValue(new Error('500'))
    const { store } = setup({})

    await user.click(screen.getByRole('combobox', { name: 'Merge strategy' }))
    await user.click(await screen.findByRole('option', { name: 'Squash and merge' }))

    expect(mocks.setMergeStrategy).toHaveBeenCalledWith(WS, 'squash')
    expect(await screen.findByText('Failed to save strategy — try again')).toBeDefined()
    await act(async () => {})
    expect(store.getState().branchReview.mergeStrategy).toBe('merge')
  })
})
