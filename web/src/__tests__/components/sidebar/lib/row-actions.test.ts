import { describe, expect, it, vi, beforeEach } from 'vitest'
import {
  performRenameRow,
  performSetWorkspaceLock,
  performImportBranches,
  performCreateFolder,
} from '@/components/sidebar/lib/row-actions'
import { useSidebarStore, getInitialState } from '@/lib/store/sidebar'
import { useFolderSignalStore } from '@/lib/store/folder-signal'
import * as api from '@/lib/api'
import * as sidebarPlacement from '@/lib/api/sidebar-placement'
import * as agentApi from '@/features/agent/api/agent-api'

vi.mock('@/features/agent/api/agent-api', async (importOriginal) => ({
  ...(await importOriginal<typeof agentApi>()),
  renameChat: vi.fn().mockResolvedValue(undefined),
}))

vi.mock('@/lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof api>()),
  renameWorkspaceBranch: vi.fn().mockResolvedValue(undefined),
  renameRepo: vi.fn().mockResolvedValue(undefined),
  setWorkspaceLock: vi.fn().mockResolvedValue(undefined),
  importBranches: vi.fn().mockResolvedValue(undefined),
}))

vi.mock('@/lib/api/sidebar-placement', async (importOriginal) => ({
  ...(await importOriginal<typeof sidebarPlacement>()),
  createFolder: vi.fn().mockResolvedValue({
    folder: {
      id: 'folder-new',
      repoId: 'repo-1',
      projectId: 'proj-1',
      name: 'New folder',
      order: 0,
    },
    shifted: [],
  }),
  placeFolder: vi.fn().mockResolvedValue({
    folder: { id: 'folder-1', repoId: 'repo-1', projectId: 'proj-1', name: 'Fixes', order: 0 },
    shifted: [],
  }),
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
          chats: [{ id: 'chat-1', repoId: 'repo-1', title: 'Fix the parser', order: 0 }],
        },
      ],
    })
    useFolderSignalStore.setState({ generations: {} })
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

  // Task 34: folders carry no dedicated push channel any more — the PATCH's
  // own {folder, shifted} response is applied to the sidebar store directly,
  // or the rename would never show up anywhere.
  it('renaming a folder applies the response to the sidebar store', async () => {
    await performRenameRow('folder-1', 'Fixes')
    expect(useSidebarStore.getState().repos[0].folders).toEqual([
      { id: 'folder-1', repoId: 'repo-1', parentId: undefined, name: 'Fixes', order: 0 },
    ])
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

  // A chat is the FOURTH id space `performRenameRow` dispatches over, and the
  // one whose fall-through was silent: it reached performRenameWorkspaceBranch,
  // found no workspace by that id, and returned — no request, no error, and the
  // name the user had just typed simply gone.
  it('renaming a chat row calls renameChat, not renameWorkspaceBranch', async () => {
    await performRenameRow('chat-1', 'Parser rewrite')
    expect(agentApi.renameChat).toHaveBeenCalledWith('ws-home', 'chat-1', 'Parser rewrite')
    expect(api.renameWorkspaceBranch).not.toHaveBeenCalled()
    expect(sidebarPlacement.placeFolder).not.toHaveBeenCalled()
  })

  // The sidebar's copy of a chat row is rebuilt from the `crowbar_chats` cache,
  // and only a reseed writes that. Without the bump the acting user waits on a
  // `title_set` frame — which only arrives if a workspace of this repo is
  // mounted — to see the name they just typed.
  it('renaming a chat bumps its repo’s tree signal so the row actually reseeds', async () => {
    await performRenameRow('chat-1', 'Parser rewrite')
    expect(useFolderSignalStore.getState().generations['repo-1']).toBe(1)
  })

  it('renaming a chat to its current title is a no-op', async () => {
    await performRenameRow('chat-1', 'Fix the parser')
    expect(agentApi.renameChat).not.toHaveBeenCalled()
    expect(useFolderSignalStore.getState().generations['repo-1']).toBeUndefined()
  })

  // The URL is repo-scoped (Task 17), so any of the REPO's own recorded
  // workspaces builds it — but never the chat's own `workspaceId`, which spec
  // §9.2 allows to name a workspace in a different repo entirely.
  it('builds the repo-scoped URL from the repo’s own workspace, not the chat’s', async () => {
    useSidebarStore.setState({
      repos: [
        {
          ...useSidebarStore.getState().repos[0],
          chats: [
            {
              id: 'chat-1',
              repoId: 'repo-1',
              title: 'Fix the parser',
              order: 0,
              workspaceId: 'ws-in-another-repo',
            },
          ],
        },
      ],
    })
    await performRenameRow('chat-1', 'Parser rewrite')
    expect(agentApi.renameChat).toHaveBeenCalledWith('ws-home', 'chat-1', 'Parser rewrite')
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
    // Applied straight to the store — no dedicated push channel exists for
    // folders any more (Task 34), so this response is the only confirmation.
    expect(useSidebarStore.getState().repos[0].folders?.map((f) => f.id)).toContain('folder-new')
  })

  it('creating a folder under the project-home row roots it at the repo instead', async () => {
    await performCreateFolder('ws-home')
    expect(sidebarPlacement.createFolder).toHaveBeenCalledWith('proj-1', 'repo-1', 'New folder', '')
  })
})
