import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, it, expect, vi } from 'vitest'
import { BranchSection } from '@/features/git/components/branch-section'
import type { GitFile } from '@/features/git/types/git-types'
import { pushChanges } from '@/features/git/api/git-remotes-api'
import { streamOperation } from '@/__tests__/__fixtures__/operations'
import { operationsState } from '@/core/entities/operations'

// Stub the heavy children so this test isolates BranchSection's own rendering.
vi.mock('@/features/git/components/commit-box', () => ({
  CommitBox: () => <div data-testid="commit-box" />,
}))
vi.mock('@/features/git/components/merge-popover', () => ({
  MergePopover: ({ trigger }: { trigger: React.ReactElement }) => trigger,
}))
// The HTTP transport: each call answers 202 with the operation it started.
const { accepted } = vi.hoisted(() => ({
  accepted: (wsId: string, opId: string) =>
    Promise.resolve({
      operation: {
        id: opId,
        kind: 'git.push',
        subject: { kind: 'workspace' as const, id: wsId },
        projectId: 'p1',
        label: 'Push epoch/first-pr',
        state: 'accepted' as const,
        items: [],
        dismissed: false,
        version: 1,
        createdAt: '2026-10-28T12:00:00Z',
        updatedAt: '2026-10-28T12:00:00Z',
      },
    }),
}))
vi.mock('@/features/git/api/git-remotes-api', () => ({
  pushChanges: vi.fn(accepted),
  pullChanges: vi.fn(accepted),
}))

const base = {
  wsId: 'w1',
  branch: 'epoch/first-pr',
  parentBranch: 'develop',
  canMergeLocally: true,
  status: 'new',
  ahead: 0,
  behind: 0,
  files: [] as GitFile[],
}

describe('BranchSection', () => {
  // The branch → parent header lives in the pill ABOVE BranchSection (rendered by
  // GitPanel, asserted in git-panel.test); BranchSection itself no longer renders
  // the branch name (ca42c0d removed the duplicate row).

  it('the commit box is always rendered, uncommitted or not', () => {
    render(
      <BranchSection {...base} files={[{ path: 'a.ts', status: 'modified', staged: false }]} />,
    )
    expect(screen.getByTestId('commit-box')).toBeDefined()
  })

  it('uncommitted → no secondary action row (the commit box is the action)', () => {
    render(
      <BranchSection {...base} files={[{ path: 'a.ts', status: 'modified', staged: false }]} />,
    )
    expect(screen.queryByRole('button', { name: /Merge into develop/ })).toBeNull()
    expect(screen.queryByRole('button', { name: /Rebase onto develop/ })).toBeNull()
  })

  it('clean + mergeable → Merge into parent', () => {
    render(<BranchSection {...base} />)
    expect(screen.getByRole('button', { name: /Merge into develop/ })).toBeDefined()
  })

  it('clean + protected → status line says so, and no dedicated secondary button (the commit box carries Pull request)', () => {
    render(<BranchSection {...base} canMergeLocally={false} />)
    expect(screen.getByText(/develop is protected/i)).toBeDefined()
    expect(screen.queryByRole('button', { name: /protected/i })).toBeNull()
    expect(screen.queryByRole('button', { name: /Merge into develop/ })).toBeNull()
  })

  it('conflicts with parent (pr-conflicts) → active "Rebase onto parent" + explain message', () => {
    render(<BranchSection {...base} status="pr-conflicts" />)
    // Active (not disabled) — the user chooses to rebase; Crowbar never does it on its own.
    const btn = screen.getByRole('button', { name: /Rebase onto develop/ })
    expect(btn).not.toBeDisabled()
    // No active "Merge into develop" affordance while conflicted.
    expect(screen.queryByRole('button', { name: /Merge into develop/ })).toBeNull()
    expect(screen.getByText(/drag it back to undo/)).toBeDefined()
  })

  it('clean + ahead → a Push secondary', () => {
    render(<BranchSection {...base} ahead={1} />)
    expect(screen.getByRole('button', { name: /Push/ })).toBeDefined()
  })

  it('diverged (ahead + behind) → shows both, and Pull wins as the action', () => {
    render(<BranchSection {...base} ahead={1} behind={1} />)
    expect(screen.getByText(/1 ahead, 1 behind/)).toBeDefined()
    // Pull-before-push: you can't push while behind, so Pull is the action.
    expect(screen.getByRole('button', { name: /Pull/ })).toBeDefined()
  })

  it('push starts a git.push operation and a failure shows its reason', async () => {
    render(<BranchSection {...base} ahead={1} />)
    fireEvent.click(screen.getByRole('button', { name: /Push/ }))

    await waitFor(() => expect(pushChanges).toHaveBeenCalled())
    const opId = vi.mocked(pushChanges).mock.calls[0][1]
    expect(operationsState().ops[opId]).toMatchObject({ kind: 'git.push' })
    expect(await screen.findByRole('button', { name: /Pushing/ })).toBeDisabled()

    act(() => {
      const held = operationsState().ops[opId]
      streamOperation({
        ...held,
        state: 'failed',
        version: held.version + 1,
        error: { code: 'internal', message: 'origin does not appear to be a git repository' },
      })
    })

    expect(await screen.findByText(/origin does not appear to be a git repository/)).toBeDefined()
  })

  // The user pushed from a terminal: nothing is left to push, so Retry has
  // nothing to do, and the reason must still be clearable.
  it('a failed push the user resolved elsewhere can be dismissed', async () => {
    const failed = {
      id: 'op-old',
      kind: 'git.push',
      subject: { kind: 'workspace' as const, id: 'w1' },
      projectId: 'p1',
      label: 'Push epoch/first-pr',
      state: 'failed' as const,
      items: [],
      dismissed: false,
      version: 3,
      createdAt: '2026-10-28T13:00:00Z',
      updatedAt: '2026-10-28T13:00:00Z',
      error: { code: 'internal' as const, message: 'rejected by the remote' },
    }
    act(() => streamOperation(failed))
    const fetchMock = vi
      .fn()
      .mockResolvedValue(
        new Response(
          JSON.stringify({ success: true, data: { ...failed, dismissed: true, version: 4 } }),
        ),
      )
    vi.stubGlobal('fetch', fetchMock)
    render(<BranchSection {...base} ahead={0} />)
    expect(screen.getByText(/rejected by the remote/)).toBeDefined()

    fireEvent.click(screen.getByRole('button', { name: 'Dismiss' }))

    await waitFor(() => expect(screen.queryByText(/rejected by the remote/)).toBeNull())
    expect(fetchMock).toHaveBeenCalledWith(
      expect.stringMatching(/\/v0\/operations\/op-old$/),
      expect.objectContaining({ method: 'DELETE' }),
    )
  })
})

afterEach(() => vi.unstubAllGlobals())
