/** The sidebar "+": forks (named, then minted) and threads off repo rows. */
import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'

const { postWorkspace, createChat, createChatWithOwnWorktree, deleteChat, toastError } = vi.hoisted(
  () => ({
    postWorkspace: vi.fn(() => Promise.resolve()),
    createChat: vi.fn(() => Promise.resolve('chat-1')),
    createChatWithOwnWorktree: vi.fn(() => Promise.resolve('chat-1')),
    deleteChat: vi.fn(() => Promise.resolve()),
    toastError: vi.fn(),
  }),
)

vi.mock('@/lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api')>()),
  postWorkspace,
}))
vi.mock('@/features/agent/api/agent-api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/features/agent/api/agent-api')>()),
  createChat,
  createChatWithOwnWorktree,
  deleteChat,
}))
vi.mock('@/features/window/stores/toast-store', () => ({
  toast: { error: toastError, success: vi.fn(), info: vi.fn() },
}))

import {
  handleCreate,
  confirmPendingCreateName,
  cancelPendingCreate,
} from '@/components/layout/create-actions'
import { handleTrash } from '@/components/layout/trash-actions'
import { getInitialState, useSidebarStore, type Repo } from '@/lib/store/sidebar'
import { getInitialRemovalState, useRemovalTrayStore } from '@/lib/store/sidebar-removal'
import { useAgentProvidersStore } from '@/features/settings/stores/agent-providers-store'
import { useSettingsStore } from '@/features/settings/store'
import { useFolderSignalStore } from '@/lib/store/folder-signal'
import { usePendingCreatesStore, getInitialPendingCreatesState } from '@/lib/store/pending-creates'
import { useHomeTreeStore } from '@/lib/store/home-tree'
import {
  windowPaneStore,
  resetWindowPaneStoreForTests,
} from '@/features/panes/stores/window-pane-store'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'

const repo = (over: Partial<Repo> = {}): Repo => ({
  id: 'r1',
  projectId: 'p1',
  name: 'crowbar',
  avatarLabel: 'C',
  avatarColor: 'bg-indigo-700',
  defaultWorkspaceId: 'home-1',
  defaultBranch: 'main',
  workspaces: [],
  folders: [],
  ...over,
})

afterEach(() => {
  // A GLOBAL store — a leaked 'terminal' default would silently arm every
  // later create in this file with a surface its own assertions never named.
  useSettingsStore.setState((state) => ({
    settings: { ...state.settings, chatIsDefaultPresentation: true },
  }))
})

beforeEach(() => {
  vi.clearAllMocks()
  useSidebarStore.setState(getInitialState())
  useRemovalTrayStore.setState(getInitialRemovalState())
  useHomeTreeStore.setState({ trees: {} })
  usePendingCreatesStore.setState(getInitialPendingCreatesState())
  // Create-workspace now needs a PROVIDER (the new atomic endpoint starts a
  // CLI, unlike the old chat-less postWorkspace) — the global provider store
  // (agent-providers-store.ts), not a per-workspace one, since there is no
  // workspace yet to scope a per-workspace read through.
  useAgentProvidersStore.setState({ status: 'ready', providers: [] })
})

/** A 'workspace' create now asks for a name before it fires (the pending
 *  row's inline input) — this drives that confirm for tests written against
 *  the old immediate-fire behavior, finding the single 'naming' entry
 *  `handleCreate` just armed exactly the way the real input would. */
function confirmArmedBranchName(name = 'typed-branch'): void {
  const armed = usePendingCreatesStore.getState().entries.find((e) => e.status === 'naming')
  if (!armed) throw new Error('confirmArmedBranchName: no naming entry is armed')
  confirmPendingCreateName(armed.tempId, name)
}

/**
 * The two verbs a chat row must NOT answer wrongly.
 *
 * `handleCreate` used to fall through to a path built for a different row
 * kind and explain itself in that kind's words — worse than doing nothing,
 * because the explanation was false. `handleTrash` used to be the same
 * shape of bug fixed the other direction — a direct `deleteChat` call with
 * no removal-tray draft at all. Addendum §2 closes THAT gap instead: a chat
 * now goes through the exact same tray every other kind already did, so its
 * delete is no longer a special case.
 */
describe('a chat row does not borrow another row kind’s refusal', () => {
  const repoWithChat = () =>
    repo({ chats: [{ id: 'c1', repoId: 'r1', title: 'a chat', order: 0 }] })

  it('handleTrash holds a chat in the removal tray — no direct deleteChat call', () => {
    useSidebarStore.setState({ repos: [repoWithChat()] })

    expect(handleTrash('c1')).toBe(true)

    expect(deleteChat).not.toHaveBeenCalled()
    const entries = useRemovalTrayStore.getState().entries
    expect(entries).toHaveLength(1)
    expect(entries[0]).toMatchObject({ kind: 'chat', id: 'c1', label: 'a chat' })
    // repo()'s default `defaultWorkspaceId` ('home-1') is the scoped
    // workspace id the DELETE request is addressed through once the hold
    // actually commits — a repo with no real `workspaces` entries.
    expect(entries[0].wsId).toBe('home-1')
    // A chat drains on the same 8s clock every non-cascading kind uses —
    // it does not wait on Cancel/Remove the way a repo/project does.
    expect(entries[0].deadlineAt).not.toBeNull()
  })

  it('handleCreate is SILENT — never the folder’s "has none to run it in" — for a bubble with no ground at all', () => {
    useSidebarStore.setState({ repos: [repoWithChat()] })
    handleCreate('c1', 'thread', vi.fn())
    expect(toastError).not.toHaveBeenCalled()
    expect(createChat).not.toHaveBeenCalled()
    expect(postWorkspace).not.toHaveBeenCalled()
  })
})

// Task 8: "create workspace" now mints the workspace AND its first chat
// atomically (POST .../chats {ownWorktree: true}) instead of the old
// chat-less postWorkspace — a bare branch row today, with a separate child
// chat row only once something ELSE later starts a conversation in it. One
// call now produces both at once (model spec §4.1, "one command replaces
// every create path").
//
// The parentId these send is a WORKSPACE id ('home-1', the repo's default
// checkout) when the fixture records no owning chat for it. The daemon
// accepts that shape as-is: it resolves the id through the workspace's own
// anchor row (validate.go resolveRow / walk.go's fork-parent walk), answers
// 201, and the new fork's git parent IS that workspace, its branch the one
// typed here (api/tests TestRegression_SidebarForkWithWorkspaceIdAsParent).
// No wire change on this side.
describe('creating a workspace off the repo-home row', () => {
  it('calls the atomic own-worktree endpoint, not postWorkspace', async () => {
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [{ id: 'claude', enabled: true }] as never,
    })
    useSidebarStore.setState({ repos: [repo()] })

    handleCreate('home-1', 'workspace', vi.fn())
    confirmArmedBranchName()
    await Promise.resolve()

    expect(createChatWithOwnWorktree).toHaveBeenCalledExactlyOnceWith(
      'p1',
      'r1',
      'claude',
      'home-1',
      'typed-branch',
    )
    expect(postWorkspace).not.toHaveBeenCalled()
  })

  // The clicked row's own id is the fallback, not the rule — see the regular-fork
  // block below, where the workspace names a real owning chat to place by. The
  // fallback is a workspace id the daemon accepts as a fork parent (201, fork
  // cut off 'ws-a' with the typed branch) — see the describe's own note.
  it('falls back to the clicked row id for a workspace that names no owning chat', async () => {
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [{ id: 'claude', enabled: true }] as never,
    })
    useSidebarStore.setState({
      repos: [repo({ workspaces: [{ id: 'ws-a', branch: 'alpha', age: '', order: 0 }] })],
    })

    handleCreate('ws-a', 'workspace', vi.fn())
    confirmArmedBranchName()
    await Promise.resolve()

    expect(createChatWithOwnWorktree).toHaveBeenCalledExactlyOnceWith(
      'p1',
      'r1',
      'claude',
      'ws-a',
      'typed-branch',
    )
  })

  it('picks the first ENABLED provider from the global provider store', async () => {
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [
        { id: 'disabled-one', enabled: false },
        { id: 'codex', enabled: true },
      ] as never,
    })
    useSidebarStore.setState({ repos: [repo()] })

    handleCreate('home-1', 'workspace', vi.fn())
    confirmArmedBranchName()
    await Promise.resolve()

    expect(createChatWithOwnWorktree).toHaveBeenCalledExactlyOnceWith(
      'p1',
      'r1',
      'codex',
      'home-1',
      'typed-branch',
    )
  })

  it('is a silent no-op with no enabled provider loaded yet', async () => {
    useAgentProvidersStore.setState({ status: 'ready', providers: [] })
    useSidebarStore.setState({ repos: [repo()] })

    handleCreate('home-1', 'workspace', vi.fn())
    await Promise.resolve()

    expect(createChatWithOwnWorktree).not.toHaveBeenCalled()
    expect(postWorkspace).not.toHaveBeenCalled()
  })

  // Regression: a burst of clicks on one row's "+" (the exact shape of "the fork
  // button does nothing" — no visible feedback between click and the row appearing
  // made a user click again) used to mint one chat AND one runner per click. Most of
  // those runners lost the concurrent-worktree-fork startup race and left a chat
  // with a real id and zero conversation, ever — permanently unresumable. One
  // request in flight per row closes this at its source.
  it('a second click while the first create is still in flight mints nothing extra', async () => {
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [{ id: 'claude', enabled: true }] as never,
    })
    useSidebarStore.setState({ repos: [repo()] })

    handleCreate('home-1', 'workspace', vi.fn())
    handleCreate('home-1', 'workspace', vi.fn())
    handleCreate('home-1', 'workspace', vi.fn())
    // The naming lock itself proves the point (only ONE naming entry armed no
    // matter how many "+" clicks landed) — confirming it is what turns that
    // into a network assertion.
    confirmArmedBranchName()
    await Promise.resolve()

    expect(createChatWithOwnWorktree).toHaveBeenCalledOnce()
  })

  it('releases the guard once the request settles, so the NEXT click is honored', async () => {
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [{ id: 'claude', enabled: true }] as never,
    })
    useSidebarStore.setState({ repos: [repo()] })

    handleCreate('home-1', 'workspace', vi.fn())
    confirmArmedBranchName('first')
    await Promise.resolve()
    await Promise.resolve()
    await Promise.resolve()
    handleCreate('home-1', 'workspace', vi.fn())
    confirmArmedBranchName('second')
    await Promise.resolve()

    expect(createChatWithOwnWorktree).toHaveBeenCalledTimes(2)
  })

  // Only one naming INPUT is ever open at once (the global "+"'s own single
  // slot, matching the old tree's `creatingChildOf`) — so this proves the
  // NETWORK half instead: once row 1's create is actually in flight (past
  // naming), opening and confirming row 2's is never blocked by it.
  it('a different row is never blocked by another row’s in-flight create', async () => {
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [{ id: 'claude', enabled: true }] as never,
    })
    useSidebarStore.setState({
      repos: [repo(), repo({ id: 'r2', projectId: 'p2', defaultWorkspaceId: 'home-2' })],
    })

    handleCreate('home-1', 'workspace', vi.fn())
    confirmArmedBranchName('first')
    handleCreate('home-2', 'workspace', vi.fn())
    confirmArmedBranchName('second')
    await Promise.resolve()

    expect(createChatWithOwnWorktree).toHaveBeenCalledTimes(2)
  })
})

/**
 * A REGULAR fork is the one row whose id is NOT the id the daemon places by.
 * Its owning chat is `type: 'chat'` (`tree/backfill.go`'s `owningChatType`) and
 * is already drawn as its own conversation beside it, so the row cannot take
 * that id the way a locked branch's does — one id would land on two rows, one
 * of them its own parent. The workspace names it instead
 * (`WorkspaceDTO.owningChatId`), and the create reads it from there.
 */
describe('creating a workspace off a REGULAR fork row', () => {
  const forkRepo = () =>
    repo({
      workspaces: [{ id: 'ws-a', branch: 'alpha', age: '', order: 0, owningChatId: 'c-owner' }],
      chats: [
        { id: 'c-owner', repoId: 'r1', type: 'chat', workspaceId: 'ws-a', title: '', order: 0 },
      ],
    })

  it('names the workspace’s OWNING CHAT, never the clicked row id', async () => {
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [{ id: 'claude', enabled: true }] as never,
    })
    useSidebarStore.setState({ repos: [forkRepo()] })

    handleCreate('ws-a', 'workspace', vi.fn())
    confirmArmedBranchName()
    await Promise.resolve()

    expect(createChatWithOwnWorktree).toHaveBeenCalledExactlyOnceWith(
      'p1',
      'r1',
      'claude',
      'c-owner',
      'typed-branch',
    )
  })

  // Live-reported, the fork half of the same "should be focused... it's just
  // adding the row" gap the thread tests pin below: a fork only knows its OWN
  // workspace id once BOTH halves of its two-aggregate placement have landed
  // (forkHasLanded's own doc) — the create response carries only the chat id
  // — so opening has to wait for that reseed rather than firing off the
  // response the way a thread's (single-aggregate) open can.
  it('opens the newly forked branch’s own chat into a pane once both halves of its placement land', async () => {
    resetWindowPaneStoreForTests()
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [{ id: 'claude', enabled: true }] as never,
    })
    useSidebarStore.setState({ repos: [forkRepo()] })
    const navigate = vi.fn(() => Promise.resolve())

    handleCreate('ws-a', 'workspace', navigate)
    confirmArmedBranchName()
    await Promise.resolve()

    // Not opened yet — the create's own promise resolved, but neither the
    // new chat nor its owning workspace has been OBSERVED in the store.
    expect(windowPaneStore.getState().panes[ROOT_PANE_ID]?.chatId).not.toBe('chat-1')

    // Both halves land: the new workspace (owned by the fresh chat) and the
    // chat itself, correctly placed under the owning chat it was forked from.
    useSidebarStore.setState({
      repos: [
        repo({
          workspaces: [
            { id: 'ws-a', branch: 'alpha', age: '', order: 0, owningChatId: 'c-owner' },
            { id: 'ws-new', branch: 'feature/x', age: '', order: 1, owningChatId: 'chat-1' },
          ],
          chats: [
            { id: 'c-owner', repoId: 'r1', workspaceId: 'ws-a', title: '', order: 0 },
            {
              id: 'chat-1',
              repoId: 'r1',
              workspaceId: 'ws-new',
              parentId: 'c-owner',
              title: '',
              order: 0,
            },
          ],
        }),
      ],
    })
    await Promise.resolve()
    await Promise.resolve()
    await Promise.resolve()

    // `ws-new` (the fresh fork's own workspace) is not the active one in this
    // test, so opening goes through the navigate-then-open path — same as a
    // thread created against a not-yet-active workspace.
    expect(navigate).toHaveBeenCalledWith({
      to: '/ide/$projectId/$repoId/$wsId',
      params: { projectId: 'p1', repoId: 'r1', wsId: 'ws-new' },
    })
    expect(windowPaneStore.getState().panes[ROOT_PANE_ID]?.chatId).toBe('chat-1')
  })

  // Regression, caught LIVE (not by any fixture here — every one above
  // happens to give `subject.id` and the owning chat the same value once you
  // trace through resolveRow, so this dimension went untested): the pending
  // row's OWN `parentId` must be the OWNING CHAT too, for the identical
  // reason the network call above already gets it right — a real sibling
  // row's `parentId` is always the parent's RENDERED (owning-chat-folded)
  // id, never the raw workspace id `resolveRow` translates the click into.
  // Using the wrong one drew the naming input as a top-level row, after
  // every other project's, instead of nested under the forked row at all.
  it('arms the naming row at the OWNING CHAT parent too, not the raw workspace id', () => {
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [{ id: 'claude', enabled: true }] as never,
    })
    useSidebarStore.setState({ repos: [forkRepo()] })

    handleCreate('ws-a', 'workspace', vi.fn())

    expect(usePendingCreatesStore.getState().entries).toMatchObject([{ parentId: 'c-owner' }])

    // Never confirmed — cancel it so this test leaves no armed
    // `createInFlight`/`armedBranchCreates` entry (module-level state
    // `beforeEach` cannot see) for a LATER test's `handleCreate('ws-a', ...)`
    // to find still locked.
    cancelPendingCreate(usePendingCreatesStore.getState().entries[0]!.tempId)
  })

  // The thread half is a different question with a different answer: it posts
  // to that workspace's chats mount, so it wants the WORKSPACE and never a
  // chat id.
  //
  // Providers come from the GLOBAL store now, not the per-workspace one this
  // used to seed — see `enabledProvider` in thread-create.ts. Seeding
  // the workspace store was itself the shape of the bug: only a MOUNTED
  // workspace ever fills that copy, so on a row the user has never opened the
  // real click found `providers: []` and returned with no request at all.
  it('its thread "+" still runs in the workspace, not in the owning chat', () => {
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [{ id: 'claude', enabled: true }] as never,
    })
    useSidebarStore.setState({ repos: [forkRepo()] })

    handleCreate('ws-a', 'thread', vi.fn())

    expect(createChat).toHaveBeenCalledExactlyOnceWith('ws-a', 'claude', 'ws-a', undefined)
  })

  // The regression that made "Thread does nothing" reproducible: a workspace
  // with NO store of its own (never mounted — exactly what a sidebar row for an
  // unopened workspace is) must still start a thread, because the provider list
  // is machine-level and has nothing to do with which workspace is on screen.
  it('starts a thread on a workspace that has never been mounted', () => {
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [{ id: 'claude', enabled: true }] as never,
    })
    useSidebarStore.setState({ repos: [forkRepo()] })

    handleCreate('ws-a', 'thread', vi.fn())

    expect(createChat).toHaveBeenCalledExactlyOnceWith('ws-a', 'claude', 'ws-a', undefined)
  })

  // A precondition that stops the click has to SAY so. Silence here is
  // indistinguishable from a dead button, which is how both create affordances
  // came to be reported as doing nothing.
  it('says why instead of silently doing nothing when no provider is enabled', () => {
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [{ id: 'claude', enabled: false }] as never,
    })
    useSidebarStore.setState({ repos: [forkRepo()] })

    handleCreate('ws-a', 'thread', vi.fn())

    expect(createChat).not.toHaveBeenCalled()
    expect(toastError).toHaveBeenCalledOnce()
  })

  it('says why instead of silently doing nothing when a fork finds no provider', () => {
    useAgentProvidersStore.setState({ status: 'ready', providers: [] as never })
    useSidebarStore.setState({ repos: [forkRepo()] })

    handleCreate('ws-a', 'workspace', vi.fn())

    expect(createChatWithOwnWorktree).not.toHaveBeenCalled()
    expect(toastError).toHaveBeenCalledOnce()
  })

  /**
   * THE OTHER HALF OF "THE FORK BUTTON DOES NOTHING": measured live, the POST
   * went out and the daemon really did mint the chat and its worktree — the
   * repo's chat count moved — and the sidebar never drew a row for it.
   *
   * `app-sync-provider.tsx`'s `openRepoTreeSubscription` reseeds `crowbar_chats`
   * on exactly one trigger, this repo's generation moving, and the only thing
   * that normally moves it is a chat frame arriving for a MOUNTED workspace of
   * the repo. Its own comment records the assumption that made that safe — "a
   * chat can only be created, renamed or moved from a surface that has that
   * workspace mounted" — which the sidebar's own Fork/Thread buttons broke.
   */
  it('bumps the repo’s tree signal after a fork so the new row is drawn', async () => {
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [{ id: 'claude', enabled: true }] as never,
    })
    useSidebarStore.setState({ repos: [forkRepo()] })
    const before = useFolderSignalStore.getState().generations['r1'] ?? 0

    handleCreate('ws-a', 'workspace', vi.fn())
    confirmArmedBranchName()
    await Promise.resolve()
    await Promise.resolve()

    expect(useFolderSignalStore.getState().generations['r1'] ?? 0).toBeGreaterThan(before)
  })

  it('bumps it after a thread too', async () => {
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [{ id: 'claude', enabled: true }] as never,
    })
    useSidebarStore.setState({ repos: [forkRepo()] })
    const before = useFolderSignalStore.getState().generations['r1'] ?? 0

    handleCreate('ws-a', 'thread', vi.fn())
    await Promise.resolve()
    await Promise.resolve()

    expect(useFolderSignalStore.getState().generations['r1'] ?? 0).toBeGreaterThan(before)
  })
})
