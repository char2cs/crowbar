import { describe, expect, it, vi, beforeEach } from 'vitest'
import {
  performRenameRow,
  performSetWorkspaceLock,
  performImportBranches,
  performCreateFolder,
  performPromoteChat,
} from '@/components/sidebar/lib/row-actions'
import { useSidebarStore, getInitialState } from '@/lib/store/sidebar'
import { useFolderSignalStore } from '@/lib/store/folder-signal'
import { toast } from '@/features/window/stores/toast-store'
import {
  destroyWorkspaceStore,
  getOrCreateWorkspaceStore,
} from '@/features/workspace/stores/workspace-store-registry'
import * as api from '@/lib/api'
import * as sidebarPlacement from '@/lib/api/sidebar-placement'
import * as agentApi from '@/features/agent/api/agent-api'

vi.mock('@/features/agent/api/agent-api', async (importOriginal) => ({
  ...(await importOriginal<typeof agentApi>()),
  renameChat: vi.fn().mockResolvedValue(undefined),
  promoteChat: vi.fn().mockResolvedValue(undefined),
}))

vi.mock('@/features/window/stores/toast-store', () => ({
  toast: { error: vi.fn(), success: vi.fn() },
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
    // The workspace-store registry is MODULE state that outlives a test
    // (sidebar-drop-policy.test.ts's own beforeEach notes the same hazard):
    // a live `agentChats.working` map left behind by one promote case would
    // silently refuse a later one that never set it up.
    destroyWorkspaceStore('ws-home')
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

  // The rename dialog seeds its input from the row's LABEL
  // (`chat.title || UNTITLED_CHAT_LABEL`), never the raw title — so for an
  // untitled chat, pressing Enter without editing sends the placeholder text
  // itself. That must stay a no-op the same way re-submitting a real title
  // does, or a blank chat's title gets permanently locked to "Untitled chat"
  // and the agent's own auto-title is rejected forever after.
  it('submitting an untitled chat’s placeholder label unedited is a no-op', async () => {
    useSidebarStore.setState({
      repos: [
        {
          ...useSidebarStore.getState().repos[0],
          chats: [{ id: 'chat-1', repoId: 'repo-1', title: '', order: 0 }],
        },
      ],
    })
    await performRenameRow('chat-1', 'Untitled chat')
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

  // Task 6: the import dialog's per-branch lock choice. `importBranches` only
  // 202s — no workspace id exists to lock yet — so the lock has to wait for the
  // branch's own workspace to land in the sidebar store (the same WS-driven
  // cache every other create relies on) before it can fire, and it does so
  // without any separate action from the caller.
  describe('locking a branch chosen at import time', () => {
    it('locks the newly-arrived workspace once its branch lands, with no separate action', async () => {
      await performImportBranches('repo-1', ['feature-a'], ['feature-a'])
      expect(api.setWorkspaceLock).not.toHaveBeenCalled()

      // Simulate the WS-driven reseed that lands the imported branch's new
      // workspace — the same store write app-sync-provider.tsx applies for any
      // create, not a raw WS frame.
      const repo = useSidebarStore.getState().repos[0]
      useSidebarStore.setState({
        repos: [
          { ...repo, workspaces: [...repo.workspaces, { id: 'ws-new', branch: 'feature-a', age: '' }] },
        ],
      })

      expect(api.setWorkspaceLock).toHaveBeenCalledWith('proj-1', 'repo-1', 'ws-new', true)
    })

    it('does not lock a branch that was imported but not chosen to be locked', async () => {
      await performImportBranches('repo-1', ['feature-a', 'feature-b'], ['feature-a'])
      const repo = useSidebarStore.getState().repos[0]
      useSidebarStore.setState({
        repos: [
          {
            ...repo,
            workspaces: [
              ...repo.workspaces,
              { id: 'ws-a', branch: 'feature-a', age: '' },
              { id: 'ws-b', branch: 'feature-b', age: '' },
            ],
          },
        ],
      })
      expect(api.setWorkspaceLock).toHaveBeenCalledTimes(1)
      expect(api.setWorkspaceLock).toHaveBeenCalledWith('proj-1', 'repo-1', 'ws-a', true)
    })

    it('leaves every workspace unlocked when no branch was chosen to be locked (default import behavior is unchanged)', async () => {
      await performImportBranches('repo-1', ['feature-a'])
      const repo = useSidebarStore.getState().repos[0]
      useSidebarStore.setState({
        repos: [
          { ...repo, workspaces: [...repo.workspaces, { id: 'ws-new', branch: 'feature-a', age: '' }] },
        ],
      })
      expect(api.setWorkspaceLock).not.toHaveBeenCalled()
    })

    // Lands the real match right after the noise so the watch's `pending` set
    // reaches zero and it unsubscribes itself before the test ends — a
    // subscription (or its 30s settle timer) surviving past the test would be
    // a leak into whatever runs next, not just an assertion gap here.
    it('an unrelated workspace change does not trigger a lock, and does not stop the watch from later locking the real one', async () => {
      await performImportBranches('repo-1', ['feature-a'], ['feature-a'])
      const repo = useSidebarStore.getState().repos[0]
      useSidebarStore.setState({
        repos: [
          { ...repo, workspaces: [...repo.workspaces, { id: 'ws-other', branch: 'other', age: '' }] },
        ],
      })
      expect(api.setWorkspaceLock).not.toHaveBeenCalled()

      const repoAfterNoise = useSidebarStore.getState().repos[0]
      useSidebarStore.setState({
        repos: [
          {
            ...repoAfterNoise,
            workspaces: [...repoAfterNoise.workspaces, { id: 'ws-new', branch: 'feature-a', age: '' }],
          },
        ],
      })
      expect(api.setWorkspaceLock).toHaveBeenCalledWith('proj-1', 'repo-1', 'ws-new', true)
    })

    // The import POST resolving is not the same moment as the workspace
    // arriving — the daemon creates it in a detached background goroutine
    // that races the HTTP round trip. A baseline snapshot taken AFTER
    // `importBranches` resolves would already see this workspace and treat it
    // as pre-existing, never locking it — the exact bug this regression test
    // catches (mirrors reparent-settle.ts's own "frame that beats the
    // response back" case, and drop-actions.ts's watchReparent(...) call
    // BEFORE the request it's watching for).
    it('locks a branch whose workspace lands in the store before the import call resolves', async () => {
      vi.mocked(api.importBranches).mockImplementationOnce(async () => {
        const repo = useSidebarStore.getState().repos[0]
        useSidebarStore.setState({
          repos: [
            { ...repo, workspaces: [...repo.workspaces, { id: 'ws-race', branch: 'feature-a', age: '' }] },
          ],
        })
      })
      await performImportBranches('repo-1', ['feature-a'], ['feature-a'])
      expect(api.setWorkspaceLock).toHaveBeenCalledWith('proj-1', 'repo-1', 'ws-race', true)
    })
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

  // §3.5/§4.2: promoting a bubble calls the repo-scoped promote endpoint,
  // built from the repo's own recorded workspace exactly as performRenameChat
  // does — never the chat's own (possibly cross-repo) workspaceId.
  it('promoting a chat calls promoteChat with the repo-scoped workspace id', async () => {
    await performPromoteChat('chat-1')
    expect(agentApi.promoteChat).toHaveBeenCalledWith('ws-home', 'chat-1')
  })

  it('promoting an unknown chat id is a no-op', async () => {
    await performPromoteChat('not-a-real-chat')
    expect(agentApi.promoteChat).not.toHaveBeenCalled()
  })

  // rows-from-repo.ts seeds every TREE chat row's `working` as always false
  // ("ALWAYS FALSE, AND NOT AN OVERSIGHT") — a promotable row can never know
  // live turn state from its own fields — so sidebar-row.tsx's render-time
  // gate cannot be the only guard. This mirrors
  // sidebar-drop-policy.test.ts's "refuses a tree chat row that IS working
  // despite its row saying false", closing the same gap for promotion:
  // promote.go respawns the CLI regardless of whether the chat is mid-turn,
  // so this must refuse BEFORE the request goes out.
  it('promoting a chat that is live mid-turn is a no-op, even though its row says working: false', async () => {
    const store = getOrCreateWorkspaceStore('ws-home')
    store.setState({
      agentChats: { ...store.getState().agentChats, working: { 'chat-1': true } },
    })
    await performPromoteChat('chat-1')
    expect(agentApi.promoteChat).not.toHaveBeenCalled()
  })

  it('promoting a chat whose live turn state says idle proceeds normally', async () => {
    const store = getOrCreateWorkspaceStore('ws-home')
    store.setState({
      agentChats: { ...store.getState().agentChats, working: { 'chat-1': false } },
    })
    await performPromoteChat('chat-1')
    expect(agentApi.promoteChat).toHaveBeenCalledWith('ws-home', 'chat-1')
  })

  // No optimistic write, matching every other perform* action's own doc
  // comments on why: the row's ownsWorktree/workspaceId only flip once the
  // daemon's broadcast/reseed lands.
  it('promoting a chat does not touch the sidebar store directly', async () => {
    const before = useSidebarStore.getState().repos[0]
    await performPromoteChat('chat-1')
    expect(useSidebarStore.getState().repos[0]).toBe(before)
  })

  it('a failed promotion surfaces a toast rather than throwing', async () => {
    vi.mocked(agentApi.promoteChat).mockRejectedValueOnce(new Error('no fork parent'))
    await expect(performPromoteChat('chat-1')).resolves.toBeUndefined()
    expect(toast.error).toHaveBeenCalledWith('no fork parent')
  })
})
