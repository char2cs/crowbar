import { act, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ReviewMergeBox } from '@/features/git/components/review-merge-box'
import type { GitStatus } from '@/features/git/types/git-types'
import { getOrCreateWorkspaceStore } from '@/features/workspace/stores/workspace-store-registry'
import type { ReviewThread } from '@/features/workspace/stores/slices/branch-review-slice'
import { useSidebarStore } from '@/lib/store/sidebar'
import type { Workspace } from '@/lib/store/sidebar'

const mocks = vi.hoisted(() => ({
  setMergeStrategy: vi.fn(),
  mergeIntoParent: vi.fn(),
  navigate: vi.fn(),
}))
vi.mock('@/features/git/api/review-api', () => ({
  setMergeStrategy: mocks.setMergeStrategy,
  mergeIntoParent: mocks.mergeIntoParent,
}))
vi.mock('@tanstack/react-router', () => ({ useNavigate: () => mocks.navigate }))

const WS = 'ws-merge'

function openThread(id: string, isResolved = false): ReviewThread {
  return {
    id,
    filePath: 'a.ts',
    lineNumber: 1,
    startLine: 1,
    endLine: 1,
    side: 'new',
    messages: [],
    isResolved,
  }
}

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
  useSidebarStore.setState({
    repos: [
      {
        id: 'r1',
        projectId: 'p1',
        workspaces: [{ id: 'parent-ws', branch: 'develop', age: '1m' }, ws],
      },
    ],
  } as never)
  const store = getOrCreateWorkspaceStore(WS)
  store.setState({
    gitStatus: { branch: 'feature', ahead: 0, behind: 0, files: [], ...opts.git },
  })
  store.setState((s) => ({
    branchReview: { ...s.branchReview, mergeStrategy: 'merge', threads: opts.threads ?? [] },
  }))
  render(<ReviewMergeBox wsId={WS} />)
  return store
}

const mergeButton = () =>
  screen.getByRole('button', { name: 'Create merge commit' }) as HTMLButtonElement

beforeEach(() => {
  vi.clearAllMocks()
  mocks.setMergeStrategy.mockResolvedValue('squash')
  mocks.mergeIntoParent.mockResolvedValue(undefined)
})

describe('ReviewMergeBox', () => {
  it('merges only when resolveBranchAction says merge, then leaves for the parent', async () => {
    const user = userEvent.setup()
    setup({})
    expect(mergeButton().disabled).toBe(false)

    await user.click(mergeButton())

    expect(mocks.mergeIntoParent).toHaveBeenCalledWith(WS, 'merge', true)
    expect(mocks.navigate).toHaveBeenCalledWith({
      to: '/ide/$projectId/$repoId/$wsId',
      params: { projectId: 'p1', repoId: 'r1', wsId: 'parent-ws' },
    })
  })

  it('keeps the workspace and stays put when delete-after-merge is unticked', async () => {
    const user = userEvent.setup()
    setup({})
    await user.click(screen.getByRole('checkbox'))
    await user.click(mergeButton())

    expect(mocks.mergeIntoParent).toHaveBeenCalledWith(WS, 'merge', false)
    expect(mocks.navigate).not.toHaveBeenCalled()
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

  it('shows the failure and re-enables Merge when the call is rejected', async () => {
    const user = userEvent.setup()
    mocks.mergeIntoParent.mockRejectedValue(new Error('500'))
    setup({})

    await user.click(mergeButton())

    expect(await screen.findByText('Merge failed — check the logs for details')).toBeDefined()
    expect(mergeButton().disabled).toBe(false)
  })

  it('saves a new strategy optimistically and reverts it when saving fails', async () => {
    const user = userEvent.setup()
    mocks.setMergeStrategy.mockRejectedValue(new Error('500'))
    const store = setup({})

    await user.click(screen.getByRole('combobox', { name: 'Merge strategy' }))
    await user.click(await screen.findByRole('option', { name: 'Squash and merge' }))

    expect(mocks.setMergeStrategy).toHaveBeenCalledWith(WS, 'squash')
    expect(await screen.findByText('Failed to save strategy — try again')).toBeDefined()
    await act(async () => {})
    expect(store.getState().branchReview.mergeStrategy).toBe('merge')
  })
})
