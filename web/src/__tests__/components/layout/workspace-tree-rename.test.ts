import { beforeEach, describe, expect, test, vi } from 'vitest'
import { useSidebarStore } from '@/lib/store/sidebar'

const renameWorkspaceBranch =
  vi.fn<(projectId: string, repoId: string, wsId: string, branch: string) => Promise<void>>()
const toastError = vi.fn<(message: string) => void>()

vi.mock('@/lib/api', () => ({
  renameWorkspaceBranch: (p: string, r: string, w: string, b: string) =>
    renameWorkspaceBranch(p, r, w, b),
  postWorkspace: vi.fn(),
  deleteWorkspace: vi.fn(),
}))

vi.mock('@/features/window/stores/toast-store', () => ({
  toast: { error: (m: string) => toastError(m) },
}))

const { performRenameWorkspaceBranch } = await import('@/components/layout/workspace-tree-actions')

function seed() {
  useSidebarStore.setState({
    repos: [
      {
        id: 'r1',
        projectId: 'p1',
        name: 'repo',
        workspaces: [
          { id: 'w1', branch: 'testing', age: 'now', status: 'new' },
          { id: 'locked', branch: 'main', age: 'now', status: 'locked' },
        ],
      },
    ],
  } as never)
}

describe('performRenameWorkspaceBranch', () => {
  beforeEach(() => {
    renameWorkspaceBranch.mockReset()
    renameWorkspaceBranch.mockResolvedValue(undefined)
    toastError.mockReset()
    seed()
  })

  test('sends the rename to the daemon with the workspace scope', async () => {
    await performRenameWorkspaceBranch('w1', 'feature/x')
    expect(renameWorkspaceBranch).toHaveBeenCalledWith('p1', 'r1', 'w1', 'feature/x')
  })

  // The bug: the sidebar used to relabel the row locally and never call the
  // daemon, so the branch snapped back on the next reseed. The row must be left
  // alone — the renamed WorkspaceDTO arrives on the WS stream.
  test('does not write the new branch into the store itself', async () => {
    await performRenameWorkspaceBranch('w1', 'feature/x')
    const ws = useSidebarStore
      .getState()
      .repos.flatMap((r) => r.workspaces)
      .find((w) => w.id === 'w1')!
    expect(ws.branch).toBe('testing')
  })

  test('a locked workspace is never renamed', async () => {
    await performRenameWorkspaceBranch('locked', 'feature/x')
    expect(renameWorkspaceBranch).not.toHaveBeenCalled()
  })

  test('renaming to the current name is a no-op', async () => {
    await performRenameWorkspaceBranch('w1', 'testing')
    expect(renameWorkspaceBranch).not.toHaveBeenCalled()
  })

  test('an unknown workspace is ignored', async () => {
    await performRenameWorkspaceBranch('nope', 'feature/x')
    expect(renameWorkspaceBranch).not.toHaveBeenCalled()
  })

  // A refusal is written for the user (the name they just typed is taken), so it
  // has to reach them rather than only the console.
  test('surfaces the daemon refusal to the user', async () => {
    renameWorkspaceBranch.mockRejectedValue(
      new Error('usecases: a workspace already exists for this branch'),
    )
    await performRenameWorkspaceBranch('w1', 'taken')
    expect(toastError).toHaveBeenCalledWith('usecases: a workspace already exists for this branch')
  })
})
