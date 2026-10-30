import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { Button } from '@/components/ui/button'
import { MergePopover } from '@/features/git/components/merge-popover'
import { useOperationsStore } from '@/lib/store/operations'
import { useSidebarStore, type Repo } from '@/lib/store/sidebar'
import { setActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'

const { patchMergeStrategy, mergeIntoParent, navigateMock } = vi.hoisted(() => ({
  patchMergeStrategy: vi.fn().mockResolvedValue('squash'),
  mergeIntoParent: vi.fn(),
  navigateMock: vi.fn(),
}))
vi.mock('@/features/git/api/review-api', () => ({
  setMergeStrategy: patchMergeStrategy,
  mergeIntoParent,
}))
vi.mock('@tanstack/react-router', () => ({ useNavigate: () => navigateMock }))

/** The fold's frame: the child workspace leaves the sidebar. */
function removeTheChild() {
  useSidebarStore.setState((s) => ({
    repos: s.repos.map((r) => ({ ...r, workspaces: r.workspaces.filter((w) => w.id !== 'w1') })),
  }))
}

/** The merge folded the child away before its operation settled. */
async function mergeFoldsTheChild() {
  removeTheChild()
  return { items: [] }
}

const settledOp = () => Object.values(useOperationsStore.getState().ops)[0]?.state

beforeEach(() => {
  vi.clearAllMocks()
  mergeIntoParent.mockImplementation(mergeFoldsTheChild)
  setActiveWorkspaceId('w1')
  useOperationsStore.setState({ ops: {}, latestFor: {} })
  useSidebarStore.setState({
    repos: [
      {
        id: 'r1',
        projectId: 'p1',
        defaultWorkspaceId: 'parent-ws',
        workspaces: [
          { id: 'parent-ws', branch: 'develop' },
          { id: 'w1', branch: 'feature/a', parentId: 'parent-ws' },
        ],
      } as unknown as Repo,
    ],
  })
})

const renderPopover = () =>
  render(
    <MergePopover wsId="w1" parentBranch="develop" trigger={<Button>Merge into develop</Button>} />,
  )

describe('MergePopover', () => {
  it('opens with the three strategies and a confirm matching the active strategy', async () => {
    const user = userEvent.setup()
    renderPopover()
    await user.click(screen.getByRole('button', { name: 'Merge into develop' }))
    expect(await screen.findByText('Create a merge commit')).toBeDefined()
    expect(screen.getByText('Squash and merge')).toBeDefined()
    expect(screen.getByText('Rebase and merge')).toBeDefined()
    expect(screen.getByRole('button', { name: 'Create merge commit' })).toBeDefined()
  })

  it('merge into parent runs an operation and closes', async () => {
    const user = userEvent.setup()
    renderPopover()
    await user.click(screen.getByRole('button', { name: 'Merge into develop' }))
    await user.click(await screen.findByRole('button', { name: 'Create merge commit' }))

    const [op] = Object.values(useOperationsStore.getState().ops)
    expect(op).toMatchObject({
      kind: 'workspace.merge_into_parent',
      subject: { kind: 'workspace', id: 'w1' },
    })
    expect(mergeIntoParent).toHaveBeenCalledWith('w1', 'merge', true, op.id)
    await waitFor(() =>
      expect(screen.queryByRole('button', { name: 'Create merge commit' })).toBeNull(),
    )
  })

  it('defaults to deleting the child + redirects to the parent once the merge settles', async () => {
    const user = userEvent.setup()
    renderPopover()
    await user.click(screen.getByRole('button', { name: 'Merge into develop' }))
    // The delete checkbox is present and on by default.
    expect(await screen.findByText('Delete this workspace after merging')).toBeDefined()
    expect(screen.getByRole('checkbox')).toBeDefined()

    await user.click(screen.getByRole('button', { name: 'Create merge commit' }))
    await waitFor(() =>
      expect(navigateMock).toHaveBeenCalledWith({
        to: '/ide/$projectId/$repoId/$wsId',
        params: { projectId: 'p1', repoId: 'r1', wsId: 'parent-ws' },
      }),
    )
  })

  it('a merge that kept the child (conflicts) does not redirect', async () => {
    mergeIntoParent.mockResolvedValue({ items: [] })
    const user = userEvent.setup()
    renderPopover()
    await user.click(screen.getByRole('button', { name: 'Merge into develop' }))
    await user.click(await screen.findByRole('button', { name: 'Create merge commit' }))

    await waitFor(() =>
      expect(Object.values(useOperationsStore.getState().ops)[0]?.state).toBe('succeeded'),
    )
    expect(navigateMock).not.toHaveBeenCalled()
  })

  it('redirects when the fold frame arrives after the operation settled', async () => {
    mergeIntoParent.mockResolvedValue({ items: [] })
    const user = userEvent.setup()
    renderPopover()
    await user.click(screen.getByRole('button', { name: 'Merge into develop' }))
    await user.click(await screen.findByRole('button', { name: 'Create merge commit' }))
    await waitFor(() => expect(settledOp()).toBe('succeeded'))
    expect(navigateMock).not.toHaveBeenCalled()

    act(() => removeTheChild())

    expect(navigateMock).toHaveBeenCalledTimes(1)
  })

  // The fold's frame lands before the terminal one and the route guard moves off
  // the vanished child at once: the user still lands on the parent, not home.
  it('lands on the parent when the child leaves while the merge still runs', async () => {
    let settle: (reply: { items: never[] }) => void = () => undefined
    mergeIntoParent.mockImplementation(() => new Promise((resolve) => (settle = resolve)))
    const user = userEvent.setup()
    renderPopover()
    await user.click(screen.getByRole('button', { name: 'Merge into develop' }))
    await user.click(await screen.findByRole('button', { name: 'Create merge commit' }))

    act(() => removeTheChild())
    act(() => setActiveWorkspaceId('fallback'))
    await act(async () => settle({ items: [] }))

    expect(navigateMock).toHaveBeenCalledTimes(1)
    expect(navigateMock).toHaveBeenCalledWith({
      to: '/ide/$projectId/$repoId/$wsId',
      params: { projectId: 'p1', repoId: 'r1', wsId: 'parent-ws' },
    })
  })

  it('does not redirect once the user has moved to another workspace', async () => {
    mergeIntoParent.mockResolvedValue({ items: [] })
    const user = userEvent.setup()
    renderPopover()
    await user.click(screen.getByRole('button', { name: 'Merge into develop' }))
    await user.click(await screen.findByRole('button', { name: 'Create merge commit' }))
    await waitFor(() => expect(settledOp()).toBe('succeeded'))

    act(() => setActiveWorkspaceId('elsewhere'))
    act(() => removeTheChild())

    expect(navigateMock).not.toHaveBeenCalled()
  })

  it('stops waiting for the fold once the popover is gone', async () => {
    mergeIntoParent.mockResolvedValue({ items: [] })
    const user = userEvent.setup()
    const { unmount } = renderPopover()
    await user.click(screen.getByRole('button', { name: 'Merge into develop' }))
    await user.click(await screen.findByRole('button', { name: 'Create merge commit' }))
    await waitFor(() => expect(settledOp()).toBe('succeeded'))

    unmount()
    act(() => removeTheChild())

    expect(navigateMock).not.toHaveBeenCalled()
  })

  it('the merge cannot start while an operation on the workspace runs', async () => {
    useOperationsStore.setState({
      ops: {
        busy: {
          ...{ id: 'busy', kind: 'workspace.sync', subject: { kind: 'workspace', id: 'w1' } },
          ...{ projectId: 'p1', label: 'Sync', state: 'running', items: [], dismissed: false },
          ...{ version: 1, createdAt: '', updatedAt: '' },
        },
      },
      latestFor: { w1: 'busy' },
    })
    renderPopover()

    expect(screen.getByRole('button', { name: 'Merge into develop' })).toBeDisabled()
  })

  it('a failed merge does not redirect', async () => {
    mergeIntoParent.mockRejectedValue(new Error('conflict'))
    const user = userEvent.setup()
    renderPopover()
    await user.click(screen.getByRole('button', { name: 'Merge into develop' }))
    await user.click(await screen.findByRole('button', { name: 'Create merge commit' }))

    await waitFor(() =>
      expect(Object.values(useOperationsStore.getState().ops)[0]?.state).toBe('failed'),
    )
    expect(navigateMock).not.toHaveBeenCalled()
  })

  it('unchecking "delete" keeps the child and does not redirect', async () => {
    const user = userEvent.setup()
    renderPopover()
    await user.click(screen.getByRole('button', { name: 'Merge into develop' }))
    // Toggle off via the label text (base-ui checkbox button doesn't toggle on a
    // direct role-click under jsdom's missing PointerEvent).
    await user.click(await screen.findByText('Delete this workspace after merging'))
    await user.click(screen.getByRole('button', { name: 'Create merge commit' }))
    expect(mergeIntoParent).toHaveBeenCalledWith('w1', 'merge', false, expect.any(String))
    expect(navigateMock).not.toHaveBeenCalled()
  })
})
