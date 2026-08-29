import { describe, expect, it, vi, beforeEach } from 'vitest'
import {
  performRenameRow,
  performSetWorkspaceLock,
  performImportBranches,
  performCreateFolder,
} from '@/components/sidebar/lib/row-actions'
import { useSidebarStore, getInitialState } from '@/lib/store/sidebar'
import * as api from '@/lib/api'
import * as sidebarPlacement from '@/lib/api/sidebar-placement'

vi.mock('@/lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof api>()),
  renameWorkspaceBranch: vi.fn().mockResolvedValue(undefined),
  renameRepo: vi.fn().mockResolvedValue(undefined),
  setWorkspaceLock: vi.fn().mockResolvedValue(undefined),
  importBranches: vi.fn().mockResolvedValue(undefined),
}))

vi.mock('@/lib/api/sidebar-placement', async (importOriginal) => ({
  ...(await importOriginal<typeof sidebarPlacement>()),
  createFolder: vi.fn().mockResolvedValue({ id: 'folder-new' }),
  placeFolder: vi.fn().mockResolvedValue(undefined),
}))

describe('row-actions', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    useSidebarStore.setState({
      ...getInitialState(),
      repos: [
        {
          id: 'repo-1',
          projectId: 'proj-1',
          name: 'repo',
          avatarLabel: 'R',
          avatarColor: 'bg-indigo-700',
          defaultWorkspaceId: 'ws-home',
          defaultBranch: 'main',
          defaultWorking: false,
          workspaces: [{ id: 'ws-1', branch: 'feature-x', age: '' }],
          folders: [{ id: 'folder-1', repoId: 'repo-1', name: 'Bugs', order: 0 }],
        },
      ],
    })
  })

  it('renaming a workspace row calls renameWorkspaceBranch with its repo/project ids', async () => {
    await performRenameRow('ws-1', 'feature-y')
    expect(api.renameWorkspaceBranch).toHaveBeenCalledWith('proj-1', 'repo-1', 'ws-1', 'feature-y')
  })

  it('renaming a folder row calls placeFolder with the new name', async () => {
    await performRenameRow('folder-1', 'Fixes')
    expect(sidebarPlacement.placeFolder).toHaveBeenCalledWith('proj-1', 'repo-1', 'folder-1', {
      name: 'Fixes',
    })
    expect(api.renameWorkspaceBranch).not.toHaveBeenCalled()
  })

  // Fix round 1: the project-home row's id is `repo.defaultWorkspaceId`,
  // never a member of `repo.workspaces` — renaming it has to name the REPO
  // (matching the deleted repo-section.tsx's "Repo rename stays on the
  // [repo name], not the branch"), not silently no-op through the
  // branch-rename path.
  it('renaming the project-home row calls renameRepo, not renameWorkspaceBranch', async () => {
    await performRenameRow('ws-home', 'renamed-repo')
    expect(api.renameRepo).toHaveBeenCalledWith('proj-1', 'repo-1', 'renamed-repo')
    expect(api.renameWorkspaceBranch).not.toHaveBeenCalled()
  })

  it('renaming the project-home row to its current name is a no-op', async () => {
    await performRenameRow('ws-home', 'repo')
    expect(api.renameRepo).not.toHaveBeenCalled()
  })

  it('locking a workspace calls setWorkspaceLock with locked: true', async () => {
    await performSetWorkspaceLock('ws-1', true)
    expect(api.setWorkspaceLock).toHaveBeenCalledWith('proj-1', 'repo-1', 'ws-1', true)
  })

  it('importing branches fires importBranches for the repo project', async () => {
    await performImportBranches('repo-1', ['feature-a', 'feature-b'])
    expect(api.importBranches).toHaveBeenCalledWith('proj-1', 'repo-1', ['feature-a', 'feature-b'])
  })

  it('importing an empty branch list is a no-op', async () => {
    await performImportBranches('repo-1', [])
    expect(api.importBranches).not.toHaveBeenCalled()
  })

  it('creating a folder under a regular workspace row passes its id straight through', async () => {
    await performCreateFolder('ws-1')
    expect(sidebarPlacement.createFolder).toHaveBeenCalledWith(
      'proj-1',
      'repo-1',
      'New folder',
      'ws-1',
    )
  })

  it('creating a folder under the project-home row roots it at the repo instead', async () => {
    await performCreateFolder('ws-home')
    expect(sidebarPlacement.createFolder).toHaveBeenCalledWith('proj-1', 'repo-1', 'New folder', '')
  })
})
