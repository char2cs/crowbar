import { beforeEach, describe, expect, test, vi } from 'vitest'
import { useSidebarStore } from '@/lib/store/sidebar'

const renameWorkspaceBranch =
  vi.fn<(projectId: string, repoId: string, wsId: string, branch: string) => Promise<void>>()
const placeFolder =
  vi.fn<
    (projectId: string, repoId: string, folderId: string, patch: { name?: string }) => Promise<void>
  >()
const toastError = vi.fn<(message: string) => void>()

vi.mock('@/lib/api', () => ({
  renameWorkspaceBranch: (p: string, r: string, w: string, b: string) =>
    renameWorkspaceBranch(p, r, w, b),
  postWorkspace: vi.fn(),
  deleteWorkspace: vi.fn(),
}))

vi.mock('@/lib/api/sidebar-placement', () => ({
  placeFolder: (p: string, r: string, f: string, patch: { name?: string }) =>
    placeFolder(p, r, f, patch),
}))

vi.mock('@/features/window/stores/toast-store', () => ({
  toast: { error: (m: string) => toastError(m) },
}))

const { performRenameWorkspaceBranch, performRenameFolder, performRenameRow } =
  await import('@/components/layout/workspace-tree-actions')

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
        folders: [{ id: 'f1', repoId: 'r1', name: 'spikes', order: 0 }],
      },
    ],
  } as never)
}

beforeEach(() => {
  renameWorkspaceBranch.mockReset()
  renameWorkspaceBranch.mockResolvedValue(undefined)
  placeFolder.mockReset()
  placeFolder.mockResolvedValue(undefined)
  toastError.mockReset()
  seed()
})

describe('performRenameWorkspaceBranch', () => {
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

/**
 * A folder rename is the folder PATCH carrying one field. It is not the branch
 * rename with a different id: nothing moves on disk, and no branch exists to
 * collide with.
 */
describe('performRenameFolder', () => {
  test('sends only the name, on the folder endpoint', async () => {
    await performRenameFolder('f1', 'experiments')
    expect(placeFolder).toHaveBeenCalledWith('p1', 'r1', 'f1', { name: 'experiments' })
  })

  test('does not write the new name into the store itself', async () => {
    await performRenameFolder('f1', 'experiments')
    const folder = useSidebarStore.getState().repos[0].folders?.[0]
    expect(folder?.name).toBe('spikes')
  })

  test('renaming to the current name is a no-op', async () => {
    await performRenameFolder('f1', 'spikes')
    expect(placeFolder).not.toHaveBeenCalled()
  })

  test('an unknown folder is ignored', async () => {
    await performRenameFolder('nope', 'experiments')
    expect(placeFolder).not.toHaveBeenCalled()
  })

  test('surfaces the daemon refusal to the user', async () => {
    placeFolder.mockRejectedValue(new Error('usecases: that folder is gone'))
    await performRenameFolder('f1', 'experiments')
    expect(toastError).toHaveBeenCalledWith('usecases: that folder is gone')
  })
})

/**
 * The tree has ONE rename gesture and one inline editor, so the id — not the row
 * that opened it — is what decides which endpoint the commit reaches.
 */
describe('performRenameRow', () => {
  test('routes a folder id to the folder endpoint', async () => {
    await performRenameRow('f1', 'experiments')
    expect(placeFolder).toHaveBeenCalledWith('p1', 'r1', 'f1', { name: 'experiments' })
    expect(renameWorkspaceBranch).not.toHaveBeenCalled()
  })

  test('routes a workspace id to the branch rename', async () => {
    await performRenameRow('w1', 'feature/x')
    expect(renameWorkspaceBranch).toHaveBeenCalledWith('p1', 'r1', 'w1', 'feature/x')
    expect(placeFolder).not.toHaveBeenCalled()
  })
})
