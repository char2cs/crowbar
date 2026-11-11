import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'
import {
  performRenameRow,
  performRenameProject,
  performSetWorkspaceLock,
  performCreateFolder,
} from '@/components/sidebar/lib/row-actions'
import {
  destroyHostRuntime,
  getHostRuntime,
  getOrCreateHostRuntime,
} from '@/features/hosts/stores/host-runtime-registry'
import {
  loadProjects,
  pairRemoteHost,
  projectOn,
  resetHostWorld,
} from '@/__tests__/__fixtures__/remote-host'
import { toast } from '@/features/window/stores/toast-store'
import { resetChatStore } from '@/__tests__/__fixtures__/agent-chat'
import * as api from '@/lib/api'
import * as sidebarPlacement from '@/lib/api/sidebar-placement'
import * as agentApi from '@/features/agent/api/agent-chat-commands'
import * as homeWorkspaceResolver from '@/features/workspace/lib/home-workspace-resolver'
import {
  expectMigratingShown,
  expectReported,
  migratingRefusal,
  refusal,
  UNWORDED,
  watchToasts,
} from '@/__tests__/__fixtures__/reports'
import { resetEntities, seedHomeTree, seedRepos } from '@/__tests__/__fixtures__/sidebar-entities'
import { readSelect, selectRepos } from '@/core/entities/select'

vi.mock('@/features/agent/api/agent-chat-commands', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/features/agent/api/agent-chat-commands')>()),
  renameChat: vi.fn().mockResolvedValue(undefined),
}))

vi.spyOn(toast, 'error').mockImplementation(() => '')
vi.spyOn(toast, 'success').mockImplementation(() => '')

vi.mock('@/lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof api>()),
  renameWorkspaceBranch: vi.fn().mockResolvedValue(undefined),
  renameRepo: vi.fn().mockResolvedValue(undefined),
  setWorkspaceLock: vi.fn().mockResolvedValue(undefined),
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
  placeHomeFolder: vi.fn().mockResolvedValue({
    folder: { id: 'home-folder-1', repoId: '', projectId: 'proj-1', name: 'Renamed', order: 0 },
    shifted: [],
  }),
}))

// Resolving the home workspace is an async fetch this file does not exercise.
vi.spyOn(homeWorkspaceResolver, 'getHomeWorkspaceId').mockReturnValue('home-ws-1')

describe('row-actions', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    // A working flag left in the ChatStore would refuse a later promote case.
    resetChatStore()
    resetEntities()
    seedRepos([
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
    ])
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

  // Folders have no push channel: the PATCH's own response is the only update.
  it('renaming a folder applies the response to the sidebar store', async () => {
    await performRenameRow('folder-1', 'Fixes')
    expect(readSelect(selectRepos)[0].folders).toEqual([
      { id: 'folder-1', repoId: 'repo-1', parentId: undefined, name: 'Fixes', order: 0 },
    ])
  })

  // The project-home row's id is `defaultWorkspaceId`, in no `repo.workspaces`:
  // renaming it names the repo rather than no-op through the branch path.
  it('renaming the project-home row calls renameRepo, not renameWorkspaceBranch', async () => {
    await performRenameRow('ws-home', 'renamed-repo')
    expect(api.renameRepo).toHaveBeenCalledWith('proj-1', 'repo-1', 'renamed-repo')
    expect(api.renameWorkspaceBranch).not.toHaveBeenCalled()
  })

  it('renaming the project-home row to its current name is a no-op', async () => {
    await performRenameRow('ws-home', 'repo')
    expect(api.renameRepo).not.toHaveBeenCalled()
  })

  // A chat id falling through to the branch path found no workspace and
  // dropped the typed name without a request or an error.
  it('renaming a chat row calls renameChat, not renameWorkspaceBranch', async () => {
    await performRenameRow('chat-1', 'Parser rewrite')
    expect(agentApi.renameChat).toHaveBeenCalledWith('ws-home', 'chat-1', 'Parser rewrite')
    expect(api.renameWorkspaceBranch).not.toHaveBeenCalled()
    expect(sidebarPlacement.placeFolder).not.toHaveBeenCalled()
  })

  it('renaming a chat to its current title is a no-op', async () => {
    await performRenameRow('chat-1', 'Fix the parser')
    expect(agentApi.renameChat).not.toHaveBeenCalled()
  })

  // The dialog seeds from the row's label, so Enter on an untitled chat sends the
  // placeholder; titling it so would lock out the agent's own auto-title.
  it('submitting an untitled chat’s placeholder label unedited is a no-op', async () => {
    seedRepos([
      {
        ...readSelect(selectRepos)[0],
        chats: [{ id: 'chat-1', repoId: 'repo-1', title: '', order: 0 }],
      },
    ])
    await performRenameRow('chat-1', 'Untitled chat')
    expect(agentApi.renameChat).not.toHaveBeenCalled()
  })

  // The URL is repo-scoped, so any of the repo's own recorded workspaces builds it — never the
  // chat's `workspaceId`, which spec §9.2 lets name a workspace in another repo.
  it('builds the repo-scoped URL from the repo’s own workspace, not the chat’s', async () => {
    seedRepos([
      {
        ...readSelect(selectRepos)[0],
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
    ])
    await performRenameRow('chat-1', 'Parser rewrite')
    expect(agentApi.renameChat).toHaveBeenCalledWith('ws-home', 'chat-1', 'Parser rewrite')
  })

  it('locking a workspace calls setWorkspaceLock with locked: true', async () => {
    await performSetWorkspaceLock('ws-1', true)
    expect(api.setWorkspaceLock).toHaveBeenCalledWith('ws-1', true)
  })

  it('a refused lock reports the daemon’s reason', async () => {
    const toasts = watchToasts()
    vi.mocked(api.setWorkspaceLock).mockRejectedValueOnce(refusal('branch is protected'))
    await expect(performSetWorkspaceLock('ws-1', true)).resolves.toBeUndefined()
    expectReported(toasts, 'Failed to update lock', 'branch is protected')
    vi.mocked(api.setWorkspaceLock).mockRejectedValueOnce(UNWORDED)
    await performSetWorkspaceLock('ws-1', true)
    expect(toasts).toHaveBeenLastCalledWith(expect.objectContaining({ description: undefined }))
  })

  it('a rename refused while the daemon migrates shows the migrating state, not a toast', async () => {
    const toasts = watchToasts()
    vi.mocked(api.renameWorkspaceBranch).mockRejectedValueOnce(migratingRefusal())
    await performRenameRow('ws-1', 'feature-y')
    expectMigratingShown(toasts)
  })

  it('creating a folder under a regular workspace row passes its id straight through', async () => {
    await performCreateFolder('ws-1')
    expect(sidebarPlacement.createFolder).toHaveBeenCalledWith(
      'proj-1',
      'repo-1',
      'New folder',
      'ws-1',
    )
    // No push channel exists for folders: this response is the only confirmation.
    expect(readSelect(selectRepos)[0].folders?.map((f) => f.id)).toContain('folder-new')
  })

  it('creating a folder under the project-home row roots it at the repo instead', async () => {
    await performCreateFolder('ws-home')
    expect(sidebarPlacement.createFolder).toHaveBeenCalledWith('proj-1', 'repo-1', 'New folder', '')
  })
})

// A branch row's id is its workspace's owning chat id: untranslated, a repo-home
// or locked-branch rename would retitle a chat. An unlocked fork is labelled by
// its chat's title, so renaming it does retitle the chat.
describe('performRenameRow — a branch row is not a chat', () => {
  beforeEach(() => {
    seedRepos([
      {
        id: 'repo-1',
        projectId: 'proj-1',
        name: 'repo',
        avatarLabel: 'R',
        avatarColor: 'bg-indigo-700',
        defaultWorkspaceId: 'ws-home',
        defaultBranch: 'main',
        workspaces: [
          {
            id: 'ws-locked',
            branch: 'develop',
            age: '',
            status: 'locked',
            owningChatId: 'locked-row',
          },
          { id: 'ws-1', branch: 'feature-x', age: '', owningChatId: 'ws-1-row' },
        ],
        chats: [
          {
            id: 'home-row',
            repoId: 'repo-1',
            ownsWorktree: true,
            workspaceId: 'ws-home',
            title: '',
            order: 0,
          },
          {
            id: 'locked-row',
            repoId: 'repo-1',
            ownsWorktree: true,
            workspaceId: 'ws-locked',
            title: '',
            order: 1,
          },
          {
            id: 'ws-1-row',
            repoId: 'repo-1',
            type: 'chat',
            workspaceId: 'ws-1',
            title: 'Fix the parser',
            order: 2,
          },
        ],
      },
    ])
  })

  it('renaming the repo-home row still renames the REPO', async () => {
    await performRenameRow('home-row', 'renamed-repo')
    expect(api.renameRepo).toHaveBeenCalledWith('proj-1', 'repo-1', 'renamed-repo')
    expect(agentApi.renameChat).not.toHaveBeenCalled()
  })

  it('renaming a LOCKED branch row is still routed as a branch rename, not a chat rename', async () => {
    // The branch rename refuses a locked workspace itself; this pins only that
    // the row is not routed to a chat rename instead.
    await performRenameRow('locked-row', 'feature-y')
    expect(api.renameWorkspaceBranch).not.toHaveBeenCalled()
    expect(agentApi.renameChat).not.toHaveBeenCalled()
  })

  it('renaming an ordinary (unlocked) fork row retitles its CHAT, not the branch', async () => {
    await performRenameRow('ws-1-row', 'New title')
    expect(agentApi.renameChat).toHaveBeenCalledWith('ws-home', 'ws-1-row', 'New title')
    expect(api.renameWorkspaceBranch).not.toHaveBeenCalled()
  })
})

// Lock and create-folder translate the owning chat id too: untranslated, an unlock
// sent nothing (a lock became a one-way door) and "New folder" fired no request.
describe('a branch row addressed by its owning chat id — lock and create-folder', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    seedRepos([
      {
        id: 'repo-1',
        projectId: 'proj-1',
        name: 'repo',
        avatarLabel: 'R',
        avatarColor: 'bg-indigo-700',
        defaultWorkspaceId: 'ws-home',
        defaultBranch: 'main',
        workspaces: [
          { id: 'ws-1', branch: 'develop', age: '', status: 'locked', owningChatId: 'ws-1-row' },
        ],
        folders: [],
        chats: [
          {
            id: 'home-row',
            repoId: 'repo-1',
            ownsWorktree: true,
            workspaceId: 'ws-home',
            title: '',
            order: 0,
          },
          {
            id: 'ws-1-row',
            repoId: 'repo-1',
            ownsWorktree: true,
            workspaceId: 'ws-1',
            title: '',
            order: 1,
          },
        ],
      },
    ])
  })

  it('unlocking a locked branch row calls setWorkspaceLock for the WORKSPACE it owns', async () => {
    await performSetWorkspaceLock('ws-1-row', false)
    expect(api.setWorkspaceLock).toHaveBeenCalledWith('ws-1', false)
  })

  it('clearing the override on a locked branch row still reaches the workspace', async () => {
    await performSetWorkspaceLock('ws-1-row', null)
    expect(api.setWorkspaceLock).toHaveBeenCalledWith('ws-1', null)
  })

  it('creating a folder under a locked branch row parents it to that CHAT id, not its workspace', async () => {
    // The translation only finds the repo: the daemon resolves `parentId` as a
    // chat or folder, so a workspace id there 404s.
    await performCreateFolder('ws-1-row')
    expect(sidebarPlacement.createFolder).toHaveBeenCalledWith(
      'proj-1',
      'repo-1',
      'New folder',
      'ws-1-row',
    )
  })

  // The home workspace is no placement target, so once translated it roots at the repo.
  it('creating a folder under the repo-home row roots it at the repo', async () => {
    await performCreateFolder('home-row')
    expect(sidebarPlacement.createFolder).toHaveBeenCalledWith('proj-1', 'repo-1', 'New folder', '')
  })
})

// A project-home row lives outside every repo, so no repo-scoped path matches it.
describe('performRenameRow — a project-home row', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    seedRepos([])
    seedHomeTree('proj-1', {
      chats: [
        {
          id: 'home-branch-chat',
          repoId: '',
          ownsWorktree: true,
          workspaceId: 'home-ws-1',
          title: '',
          order: 0,
        },
        { id: 'home-chat-1', repoId: '', title: 'Old title', order: 1 },
      ],
      folders: [{ id: 'home-folder-1', repoId: '', name: 'Old name', order: 0 }],
    })
  })

  it('renames a project-home chat via the home workspace scope, not a repo', async () => {
    await performRenameRow('home-chat-1', 'New title')
    expect(agentApi.renameChat).toHaveBeenCalledWith('home-ws-1', 'home-chat-1', 'New title')
  })

  it('a no-op home chat rename (same title) sends nothing', async () => {
    await performRenameRow('home-chat-1', 'Old title')
    expect(agentApi.renameChat).not.toHaveBeenCalled()
  })

  it('renames a project-home folder via the home-scoped placement endpoint', async () => {
    await performRenameRow('home-folder-1', 'New name')
    expect(sidebarPlacement.placeHomeFolder).toHaveBeenCalledWith('proj-1', 'home-folder-1', {
      name: 'New name',
    })
    expect(sidebarPlacement.placeFolder).not.toHaveBeenCalled()
  })

  it('a no-op home folder rename (same name) sends nothing', async () => {
    await performRenameRow('home-folder-1', 'Old name')
    expect(sidebarPlacement.placeHomeFolder).not.toHaveBeenCalled()
  })
})

// A row action on a remote project names the host that failed it (by its name), records what
// that says of the host, and is never sent again on its own.
describe('a row action its host fails', () => {
  const statusOf = (hostId: string) => getHostRuntime(hostId)?.getState().status

  beforeEach(() => {
    vi.clearAllMocks()
    resetHostWorld()
    pairRemoteHost('h1', 'Studio Mac')
    loadProjects(projectOn('p-remote', 'h1'))
    getOrCreateHostRuntime('h1').getState().setStatus('up')
    // The desktop app: the only one that reaches a remote host.
    Object.assign(window, { __TAURI_INTERNALS__: {} })
  })

  afterEach(() => {
    delete (window as { __TAURI_INTERNALS__?: unknown }).__TAURI_INTERNALS__
    vi.unstubAllGlobals()
    destroyHostRuntime('h1')
    resetHostWorld()
  })

  it.each([
    [
      'a changed identity',
      502,
      { 'x-crowbar-proxy': 'host-identity-changed' },
      'Studio Mac is presenting a different identity — confirm it before pairing again',
    ],
    ['a refused credential (401)', 401, {}, 'Studio Mac needs to be paired again'],
  ])(
    'says %s needs pairing again, marks the host for repair, and asks it once',
    async (_, status, headers, message) => {
      const json = async () => ({ error: 'client is not paired' })
      const fetch = vi
        .fn()
        .mockResolvedValue({ ok: false, status, headers: new Headers(headers), json })
      vi.stubGlobal('fetch', fetch)
      const toasts = watchToasts()

      await performRenameProject('p-remote', 'renamed')

      expect(fetch).toHaveBeenCalledOnce()
      expectReported(toasts, 'Failed to rename project', message)
      expect(statusOf('h1')).toBe('needs-repair')
    },
  )

  it('says the projects are still loading, never why the host is unknown inside', async () => {
    resetHostWorld()
    pairRemoteHost('h1', 'Studio Mac')
    const fetch = vi.fn()
    vi.stubGlobal('fetch', fetch)
    const toasts = watchToasts()

    await performRenameProject('p-remote', 'renamed')

    expectReported(toasts, 'Failed to rename project', 'Still loading your projects')
    expect(fetch).not.toHaveBeenCalled()
  })
})
