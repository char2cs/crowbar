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

// `resolveHomeRowScope` needs a resolved home workspace id — real elsewhere
// in this file's fixtures, but real resolution is an async fetch this file
// has no business exercising.
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

  // Task 34: folders carry no dedicated push channel any more — the PATCH's
  // own {folder, shifted} response is applied to the sidebar store directly,
  // or the rename would never show up anywhere.
  it('renaming a folder applies the response to the sidebar store', async () => {
    await performRenameRow('folder-1', 'Fixes')
    expect(readSelect(selectRepos)[0].folders).toEqual([
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

  it('renaming a chat to its current title is a no-op', async () => {
    await performRenameRow('chat-1', 'Fix the parser')
    expect(agentApi.renameChat).not.toHaveBeenCalled()
  })

  // The rename dialog seeds its input from the row's LABEL
  // (`chat.title || UNTITLED_CHAT_LABEL`), never the raw title — so for an
  // untitled chat, pressing Enter without editing sends the placeholder text
  // itself. That must stay a no-op the same way re-submitting a real title
  // does, or a blank chat's title gets permanently locked to "Untitled chat"
  // and the agent's own auto-title is rejected forever after.
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

  // The URL is repo-scoped (Task 17), so any of the REPO's own recorded
  // workspaces builds it — but never the chat's own `workspaceId`, which spec
  // §9.2 allows to name a workspace in a different repo entirely.
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
    // Applied straight to the store — no dedicated push channel exists for
    // folders any more (Task 34), so this response is the only confirmation.
    expect(readSelect(selectRepos)[0].folders?.map((f) => f.id)).toContain('folder-new')
  })

  it('creating a folder under the project-home row roots it at the repo instead', async () => {
    await performCreateFolder('ws-home')
    expect(sidebarPlacement.createFolder).toHaveBeenCalledWith('proj-1', 'repo-1', 'New folder', '')
  })
})

/**
 * A FIFTH id space, and the one that overlaps a fourth: a `branch` row's id is
 * the id of the CHAT that owns its workspace (`deriveRows`). Without
 * translating first, the repo-home row stops matching `defaultWorkspaceId`,
 * falls into the chat branch, and renaming the repo silently retitles a
 * conversation instead — a locked branch's rename must still move the git
 * branch, not retitle its own row as if it were a chat.
 *
 * An ORDINARY (unlocked) fork's row is the one case that changed with rule
 * 6: its label is now its owning chat's title, not its branch, so renaming
 * it retitles the chat — the opposite of what this same row did before every
 * workspace-owning row folded the same way.
 */
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
    // `performRenameWorkspaceBranch` itself refuses a locked workspace before
    // it ever reaches the API (its own guard, unrelated to this fix) — what
    // this pins is the ROUTING decision one level up: a locked row's rename
    // must not fall through to retitling the chat instead.
    await performRenameRow('locked-row', 'feature-y')
    expect(api.renameWorkspaceBranch).not.toHaveBeenCalled()
    expect(agentApi.renameChat).not.toHaveBeenCalled()
  })

  // Rule 6: an ordinary (unlocked) fork's row is now labelled by its owning
  // chat's title, not its branch — so renaming it retitles the CHAT, the same
  // as any other chat row, and no longer moves the git branch at all.
  it('renaming an ordinary (unlocked) fork row retitles its CHAT, not the branch', async () => {
    await performRenameRow('ws-1-row', 'New title')
    expect(agentApi.renameChat).toHaveBeenCalledWith('ws-home', 'ws-1-row', 'New title')
    expect(api.renameWorkspaceBranch).not.toHaveBeenCalled()
  })
})

/**
 * The same fifth id space, at the two verbs the first fix round missed.
 *
 * `performRenameRow` was translated; `performSetWorkspaceLock` and
 * `performCreateFolder` were not, and both match a row id against exactly the
 * spaces a branch row is NOT in. Lock/unlock resolved no repo and sent nothing —
 * which, with `DeleteCascade` refusing a locked workspace, made a lock a
 * one-way door — and "New folder" on the repo home or any locked branch fired
 * no request at all.
 */
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
    // The workspace-id translation exists only to find the owning repo and to
    // detect the repo-home special case below — `POST .../chats/folders`
    // resolves its `parentId` as a chat (or folder), so the value actually
    // sent must stay the untranslated row id. Sending the workspace id here
    // 404s on the daemon ("agentchat: get chat: not found"), caught live.
    await performCreateFolder('ws-1-row')
    expect(sidebarPlacement.createFolder).toHaveBeenCalledWith(
      'proj-1',
      'repo-1',
      'New folder',
      'ws-1-row',
    )
  })

  // The repo-home row is the same translation with a second step behind it: the
  // home workspace is not a placement target the daemon knows, so it normalises
  // to the repo root — which it can only do once the row id has become
  // `defaultWorkspaceId` again.
  it('creating a folder under the repo-home row roots it at the repo', async () => {
    await performCreateFolder('home-row')
    expect(sidebarPlacement.createFolder).toHaveBeenCalledWith('proj-1', 'repo-1', 'New folder', '')
  })
})

/**
 * A SIXTH id space `performRenameRow` has to know about: a project-home row
 * lives outside every repo entirely (home rides no repo), so none of the
 * repo-scoped branches above ever match one — renaming a home chat or
 * folder silently no-op'd before this, caught live as "can't rename
 * folders."
 */
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

      await performRenameProject('p-remote', 'renamed')

      expect(fetch).toHaveBeenCalledOnce()
      expect(vi.mocked(toast.error).mock.calls[0]?.[0]).toBe(message)
      expect(statusOf('h1')).toBe('needs-repair')
    },
  )

  it('says the projects are still loading, never why the host is unknown inside', async () => {
    resetHostWorld()
    pairRemoteHost('h1', 'Studio Mac')
    const fetch = vi.fn()
    vi.stubGlobal('fetch', fetch)

    await performRenameProject('p-remote', 'renamed')

    expect(toast.error).toHaveBeenCalledExactlyOnceWith(
      'Still loading your projects',
      'Try again in a moment.',
    )
    expect(fetch).not.toHaveBeenCalled()
  })
})
