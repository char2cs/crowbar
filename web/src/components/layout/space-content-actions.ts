import type { useNavigate } from '@tanstack/react-router'
import { useSidebarStore, type Chat, type Repo } from '@/lib/store/sidebar'
import { useRemovalTrayStore } from '@/lib/store/sidebar-removal'
import { useProjectDataStore, EMPTY_PROJECTS } from '@/lib/store/projects'
import { dataOf } from '@/lib/loadable'
import { planRemoval, type DragSubject } from './removal-plan'
import { createChat, createChatWithOwnWorktree } from '@/features/agent/api/agent-api'
import { getActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'
import { useAgentProvidersStore } from '@/features/settings/stores/agent-providers-store'
import { useFolderSignalStore } from '@/lib/store/folder-signal'
import { usePendingCreatesStore } from '@/lib/store/pending-creates'
import { workspaceIdOfBranchRow } from '@/components/sidebar/lib/branch-row-id'
import { toast } from '@/features/window/stores/toast-store'
import { openChatInOwnPane } from '@/components/sidebar/lib/drop-actions'
import { resolveHomeRowScope, getHomeTree, useHomeTreeStore } from '@/lib/store/home-tree'
import { rowsFromHome } from '@/components/sidebar/lib/rows-from-home'
import { rowsFromRepo, resolveHomeOwnerId } from '@/components/sidebar/lib/rows-from-repo'
import type { SidebarRow as SidebarRowType } from '@/components/sidebar/types/sidebar-row'

/** What `id` resolves to: its owning repo, and the subject a drag/removal call needs. */
export interface ResolvedRow {
  repo: Repo
  subject: DragSubject
}

type NavigateFn = ReturnType<typeof useNavigate>

/**
 * Find `id` among the repo's CHAT rows.
 *
 * Deliberately its own resolver rather than a fourth branch inside
 * `resolveRow` below: that one answers with a `DragSubject`, whose `kind` is
 * the closed `DropKind` union (`workspace | folder | repo | project`) that the
 * drag matrix, `planRemoval` and `RemovalDraft` all switch on exhaustively.
 * Widening that union to admit a chat would change four surfaces that have
 * nothing to do with clicking a row, so a chat answers here instead and
 * `resolveRow` keeps exactly the contract it had.
 *
 * Callers must consult THIS FIRST. `resolveRow` cannot see a chat, and every
 * one of its callers treats "not found" as "do nothing" — which is how a chat
 * row came to look like every other row while silently doing nothing.
 *
 * AND it deliberately does NOT match a `branch` row, even though one lives in
 * the chat id space: `rows-from-repo.ts` gives EVERY workspace-owning row —
 * a locked branch, a repo home, and an ordinary fork or forked thread alike —
 * the id of the chat that owns its workspace, because that is what the daemon
 * places by. Such a row is a WORKSPACE — `resolveRow` answers for it, via
 * `workspaceIdOfBranchRow`. Matching it here sent every verb down the chat
 * path, which is not a hypothetical: it made a locked branch's "+" silently
 * inert, pointed its trash at `deleteChat`, and turned renaming the repo-home
 * row into retitling a conversation. A new caller must handle both clauses.
 */
export function resolveChatRow(
  repos: readonly Repo[],
  id: string,
): { repo: Repo; chat: Chat } | null {
  // A workspace-owning row lives in the chat id space but is NOT a chat row —
  // `rows-from-repo.ts` gives that workspace's row this id precisely so the
  // daemon can place under it. Matching it below would send every verb down
  // the chat path: "+" would go silently inert, trash would delete the
  // branch's own row, and rename would retitle it instead of moving the git
  // branch. `workspaceIdOfBranchRow` is the one place that already knows every
  // shape this id can take (locked, home, or an ordinary fold) — checked once,
  // up front, rather than re-deriving its own `type === 'branch'` shortcut
  // here, which stopped being able to tell a folded fork's row apart from a
  // real conversation the moment it started sharing that id space too.
  if (workspaceIdOfBranchRow(repos, id) !== null) return null
  for (const repo of repos) {
    const chat = repo.chats?.find((c) => c.id === id)
    if (chat) return { repo, chat }
  }
  return null
}

/**
 * The workspace a chat row OPENS, or null when it opens none.
 *
 * A worktree chat (§3.1) names the workspace it owns and navigating to it is
 * exactly what clicking a branch row does. A bubble names none — it borrows an
 * ancestor's ground — and there is nothing to navigate to.
 *
 * The named workspace must belong to THIS repo. Spec §9.2 makes a repo's chats
 * an open set (a bubble moved across repos keeps ancestors in the repo it
 * left), so a `workspaceId` pointing outside is an ordinary state, not
 * corruption — and routing to /ide/:p/:r/:ws with a ws that is not under :r
 * would be a URL nothing resolves.
 */
function openableWorkspaceOf(repo: Repo, chat: Chat): string | null {
  const wsId = chat.workspaceId
  if (!wsId) return null
  if (wsId === repo.defaultWorkspaceId) return wsId
  return repo.workspaces.some((w) => w.id === wsId) ? wsId : null
}

/**
 * Find `id` — a workspace, a folder, or a repo's own default workspace —
 * across every visible repo, regardless of which project's space panel
 * rendered it. Extracted verbatim from sidebar-tree-panel.tsx (Task 8/29): a
 * row's owning repo is never ambiguous by project, so this needs no
 * per-project variant.
 *
 * NOT a chat resolver — see `resolveChatRow` above for why, and call that
 * first.
 */
export function resolveRow(repos: readonly Repo[], id: string): ResolvedRow | null {
  // A branch row is addressed by its owning chat's id, but everything downstream
  // of here — the drag matrix, `planRemoval`, `RemovalDraft` — is about the
  // WORKSPACE. Translate once, at the boundary, so none of them has to know two
  // id spaces exist.
  const rowId = workspaceIdOfBranchRow(repos, id) ?? id
  for (const repo of repos) {
    const ws = repo.workspaces.find((w) => w.id === rowId)
    if (ws) {
      return {
        repo,
        subject: {
          kind: 'workspace',
          id: rowId,
          repoId: repo.id,
          locked: ws.status === 'locked',
          parentId: ws.parentId,
        },
      }
    }
    const folder = repo.folders?.find((f) => f.id === id)
    if (folder) {
      return { repo, subject: { kind: 'folder', id, repoId: repo.id, parentId: folder.parentId } }
    }
    if (repo.defaultWorkspaceId === rowId) {
      // Not a real drag/removal subject — `draftFor` finds no matching row in
      // `repo.workspaces` for this id and returns null, which is what makes
      // trashing the repo-home row a safe no-op below rather than a delete.
      return { repo, subject: { kind: 'workspace', id: rowId, repoId: repo.id } }
    }
  }
  return null
}

/** A `SidebarRow` good for exactly one thing: naming a chat and its
 *  workspace for `openChatInOwnPane`, which reads only those two fields
 *  (`subject.id`, `subject.workspaceId`). Every other field here is inert
 *  filler required by the type, not real row data — never hand this to
 *  anything that renders or drags a row. */
function paneOpenSubject(chatId: string, workspaceId: string): SidebarRowType {
  return {
    id: chatId,
    kind: 'chat',
    parentId: null,
    order: 0,
    label: '',
    ownsWorktree: false,
    workspaceId,
    working: false,
    hasView: false,
  }
}

/**
 * Open `chatId` (which runs in `workspaceId`) the way a click does — spec
 * §8.4, "clicking a chat in the tree makes its own view": `openChatInOwnPane`
 * reveals it if it is already up, fills an empty pane if there is one, and
 * otherwise gives it a brand-new pane of its own.
 *
 * It is deliberately NOT `openChatIntoPane(…, activePaneId, 'center')` any
 * more. That call handed the click the DRAG-AND-DROP rules — a synthetic
 * "you dropped this exactly on the active pane" — whose occupied-pane branch
 * is a merge: a split carved out of the active pane and both chats grouped
 * into one Recents entry. Clicking a second row then read as appending a chat
 * to the view you were in, which is what it was reported as. Merging stays a
 * drag-and-drop gesture; see `openChatInOwnPane`'s own doc.
 *
 * Only reachable when `workspaceId` IS ALREADY the active workspace:
 * `openChatInOwnPane` itself refuses otherwise (the same documented guard
 * `openChatIntoPane` carries — no chatId->workspace resolution exists yet on
 * the render side for an off-screen workspace). A row naming a workspace that
 * is not yet active goes through `navigateThenOpenChat` below instead, which
 * waits for it to become active first rather than racing it.
 */
function openChatInOwnView(chatId: string, workspaceId: string): boolean {
  if (workspaceId !== getActiveWorkspaceId()) return false
  openChatInOwnPane(paneOpenSubject(chatId, workspaceId))
  return true
}

/**
 * Poll until `wsId` becomes the active workspace, or give up.
 *
 * `setActiveWorkspaceId` fires from `WorkspaceView`'s own `useEffect`
 * (workspace-view.tsx) — a render-and-effect cycle that a route change's own
 * promise does not wait on, and only runs once that workspace's view has
 * actually (re)mounted as the active one. `_activeWorkspaceId` is a plain
 * module variable, not a store, so there is nothing to subscribe to; this
 * polls the one function that reads it instead. Bounded so a workspace that
 * never activates (an id the route guard redirects away from) cannot hang a
 * click forever — matches the 2s the app already gives similar
 * activation-effect races elsewhere.
 */
function waitForActiveWorkspace(wsId: string, timeoutMs = 2000): Promise<boolean> {
  if (getActiveWorkspaceId() === wsId) return Promise.resolve(true)
  return new Promise((resolve) => {
    const start = Date.now()
    const tick = () => {
      if (getActiveWorkspaceId() === wsId) {
        resolve(true)
        return
      }
      if (Date.now() - start >= timeoutMs) {
        resolve(false)
        return
      }
      requestAnimationFrame(tick)
    }
    requestAnimationFrame(tick)
  })
}

/**
 * Navigate to `wsId`'s own route, then open `chatId` into its own view once
 * the workspace has actually finished becoming active.
 *
 * This is the sequencing `openChatInOwnView` alone cannot do: a bare
 * `navigate()` only changes the URL, and clicking a workspace that was not
 * already active used to stop there — the click looked like it did nothing,
 * because nothing ever wrote a chat into a pane. `openChatInOwnPane`'s own
 * guard against an off-screen workspace's chat is exactly right; the fix is
 * to wait until the workspace is no longer off-screen, not to bypass it.
 */
async function navigateThenOpenChat(
  navigate: NavigateFn,
  params: { projectId: string; repoId: string; wsId: string },
  chatId: string,
): Promise<void> {
  await navigate({ to: '/ide/$projectId/$repoId/$wsId', params })
  const becameActive = await waitForActiveWorkspace(params.wsId)
  if (!becameActive) return
  openChatInOwnPane(paneOpenSubject(chatId, params.wsId))
}

/**
 * Opens a row: into a pane (a real, unlocked chat/workspace row), or toggles
 * a fold (a container — a folder, the repo home, or a locked/protected
 * branch, addendum §3: "a project, a repo, and a locked/protected branch
 * never open a pane and never open a workspace of their own on click...
 * they contain rows; they are not rows you check out into").
 *
 * `repos` is the caller's removal-tray-filtered list (what is actually
 * rendered), not the raw store — a row already hidden pending removal must
 * not be openable.
 *
 * A CHAT row answers this the same way every other row does, chosen by the
 * one fact §3.1 distinguishes chats on:
 *
 *   - a WORKTREE chat owns a workspace, so it opens into the active pane
 *     (spec §8.4) — the same view clicking the branch row for that workspace
 *     opens, since both name the same conversation;
 *   - a BUBBLE owns none, so it folds, like every other container that has
 *     no destination of its own. Opening a bubble into a pane needs its
 *     ground workspace resolved by walking up to the nearest owning
 *     ancestor (§3.2) — not built yet (see `handleCreate`'s own note on the
 *     same gap) — so this is an honest fold, not a placeholder.
 *
 * A project-home row (chat or folder) is resolved FIRST, against every
 * visible project's home tree rather than `repos` — see `resolveHomeRowScope`
 * (lib/store/home-tree.ts), the one shared implementation every caller that
 * needs to tell a home row apart from a repo one reuses.
 */
export function handleOpen(id: string, repos: readonly Repo[], navigate: NavigateFn): void {
  const homeRow = resolveHomeRowScope(id)
  if (homeRow) {
    if (homeRow.kind === 'folder') {
      useSidebarStore.getState().toggleChatRow(id)
      return
    }
    void openHomeChat(homeRow.projectId, homeRow.homeWorkspaceId, id, navigate)
    return
  }

  const chatRow = resolveChatRow(repos, id)
  if (chatRow) {
    const wsId = openableWorkspaceOf(chatRow.repo, chatRow.chat)
    const projectId = chatRow.repo.projectId
    if (!wsId || !projectId) {
      useSidebarStore.getState().toggleChatRow(id)
      return
    }
    if (openChatInOwnView(id, wsId)) return
    void navigateThenOpenChat(navigate, { projectId, repoId: chatRow.repo.id, wsId }, id)
    return
  }

  const found = resolveRow(repos, id)
  if (!found) return
  // A folder has no destination — the row body toggles it, same as the
  // tree it replaces.
  if (found.subject.kind === 'folder') {
    useSidebarStore.getState().toggleChatRow(id)
    return
  }
  if (!found.repo.projectId) return
  // Addendum §3: the repo-home row (no `locked` on its subject at all — see
  // `resolveRow`) and a locked/protected branch never open anything of
  // their own on click; they are containers, not workspaces you check out
  // into. Fold, exactly like a folder.
  const isRepoHome = found.subject.id === found.repo.defaultWorkspaceId
  if (isRepoHome || found.subject.locked) {
    useSidebarStore.getState().toggleChatRow(id)
    return
  }
  // A real, unlocked workspace row IS a chat — the one that owns it — so it
  // opens the same way clicking that chat's own bubble would (spec §8.4).
  // The row itself is addressed by the WORKSPACE's id for a regular fork
  // (`rows-from-repo.ts`'s own note on why); its owning conversation is a
  // separate id, read off the `Workspace` record the same way
  // `handleCreate` already does.
  const owningChatId = found.repo.workspaces.find((w) => w.id === found.subject.id)?.owningChatId
  if (owningChatId && openChatInOwnView(owningChatId, found.subject.id)) return
  // `subject.id`, never the row's — a branch row is addressed by its owning
  // chat, and only `resolveRow` knows which workspace that names.
  const params = { projectId: found.repo.projectId, repoId: found.repo.id, wsId: found.subject.id }
  if (owningChatId) {
    void navigateThenOpenChat(navigate, params, owningChatId)
    return
  }
  void navigate({ to: '/ide/$projectId/$repoId/$wsId', params })
}

/**
 * Trashes a row — a chat, workspace, folder, or repo — through the SAME
 * removal tray every kind uses (`planRemoval`/`RemovalDraft`, addendum §2's
 * `chat` kind included now). Reads the store's raw, current repos (not a
 * caller-supplied filtered snapshot): the true current state, not the UI's
 * already-hidden-pending-removal overlay.
 *
 * A chat used to bypass the tray entirely here — a direct `deleteChat` call
 * with no hold, no 8s undo, nothing to cancel. That is exactly what a drag
 * dropped onto the trash target must never do (addendum §2's whole point is
 * routing this gesture into the SAME safety net every other kind already
 * gets), so this is now the one path for both: the row-level trash call this
 * function always was, and `use-sidebar-drag.ts`'s drag-to-trash commit,
 * which calls this directly per dragged id.
 *
 * Returns whether anything was actually held. `false` covers: a chat/row the
 * live store no longer recognises, a repo-home id (resolves to a `workspace`
 * subject naming no row in `repo.workspaces` — repo deletion gets its own
 * confirmation flow in Part H and is not reachable from a row's trash yet),
 * a user-locked, non-home workspace (`planRemoval`'s `draftFor` refuses
 * one — the daemon would refuse the delete too, so the tray must never
 * accept one and promise otherwise), and — checked FIRST, below — a
 * project-home row.
 *
 * A home row is resolved FIRST, against every visible project's home tree,
 * for the same reason `handleOpen`/`handleCreate` already check
 * `resolveHomeRowScope` before anything repo-scoped: `resolveRow`'s
 * repo-scoped walk can find a FALSE match for one. The daemon's `ListInRepo`
 * never actually filters by the repo id in its own URL (`fetchFolders`'s own
 * doc — a known, unfixed backend leniency), so a home folder bleeds into
 * every REPO's own folder list too, stamped with THAT repo's id. Trusting
 * that match here is what silently deleted a home folder through a
 * repo-scoped DELETE that had no business resolving it at all — caught
 * live, dragging a home folder onto the trash target. `planRemoval`'s
 * `draftFor` now builds a real removal draft for a home chat/folder once
 * `resolveHomeRowScope` names it; the subject built below never falls
 * through to `resolveRow`'s repo-scoped (and bleed-prone) folder lookup for
 * one.
 */
export function handleTrash(id: string): boolean {
  const currentRepos = useSidebarStore.getState().repos
  const homeRow = resolveHomeRowScope(id)
  // Checked BEFORE resolveChatRow/resolveRow, which cannot see a home row at
  // all (and, for a folder, would risk the bleed-prone false match above).
  const chatRow = homeRow ? null : resolveChatRow(currentRepos, id)
  const subject: DragSubject | null = homeRow
    ? { kind: homeRow.kind, id }
    : chatRow
      ? { kind: 'chat', id, repoId: chatRow.repo.id }
      : (resolveRow(currentRepos, id)?.subject ?? null)
  if (!subject) return false
  const projects = dataOf(useProjectDataStore.getState().data) ?? EMPTY_PROJECTS
  const drafts = planRemoval([subject], currentRepos, projects)
  if (drafts.length === 0) return false
  useRemovalTrayStore.getState().hold(drafts)
  return true
}

/**
 * Trashes a whole PROJECT via the same removal tray a row's trash uses —
 * spec §9: "every row that owns something carries a trash: chats,
 * workspaces, folders, repos, and the space header for the project."
 *
 * Deliberately not routed through `DeleteConfirmDialog` the way a row's
 * trash is: `planRemoval`'s project draft already hides the project's row
 * AND every repo under it, and `RemovalTray` pops `RemovalConfirmDialog`
 * for exactly the two cascading kinds (`repo`, `project`) before it commits
 * — so a project already gets the "confirm names what goes" step, from the
 * surface that owns it, and a second dialog in front would ask twice.
 *
 * Returns whether anything was actually held, so the caller can say
 * something rather than silently doing nothing (`draftFor` returns null for
 * a project id no loaded project claims).
 */
export function handleTrashProject(projectId: string): boolean {
  const currentRepos = useSidebarStore.getState().repos
  const projects = dataOf(useProjectDataStore.getState().data) ?? EMPTY_PROJECTS
  const drafts = planRemoval([{ kind: 'project', id: projectId }], currentRepos, projects)
  if (drafts.length === 0) return false
  useRemovalTrayStore.getState().hold(drafts)
  return true
}

/**
 * Trashes a whole REPO — spec §9's "repos" clause, the one `handleTrash`
 * itself deliberately can't reach: the repo's own home row resolves to a
 * `workspace` subject (its default branch), which `handleTrash` refuses
 * (sidebar-row.tsx's own doc on why that row excludes the X control) rather
 * than silently deleting just that one branch out from under the repo it
 * belongs to. `kind: 'repo'` is the real subject. Mirrors
 * `handleTrashProject` exactly, one kind over — `RemovalConfirmDialog`
 * already has its own cascading-confirm copy for `repo`, same as `project`.
 */
export function handleTrashRepo(repoId: string): boolean {
  const currentRepos = useSidebarStore.getState().repos
  const projects = dataOf(useProjectDataStore.getState().data) ?? EMPTY_PROJECTS
  const drafts = planRemoval([{ kind: 'repo', id: repoId }], currentRepos, projects)
  if (drafts.length === 0) return false
  useRemovalTrayStore.getState().hold(drafts)
  return true
}

// Keys currently mid-request, so a rapid-fire burst of clicks on one row's "+" mints
// AT MOST ONE chat instead of one per click. This used to be reachable for real: with
// no visible feedback between click and the row appearing (the bug `announceTreeChange`
// exists to fix), a user who saw nothing happen clicked again — and each click was a
// genuine, separate POST the daemon happily minted a chat and a runner for. Most of
// those runners then lost the startup race spawn.go's own doc describes (concurrent
// worktree forks off the same parent are exactly the contention that race needs),
// leaving a pile of chats with a real id and zero conversation, ever — permanently
// unresumable, forever re-erroring the moment anything opens them. One request in
// flight per (kind, parentId) closes the hole at its source rather than papering over
// the mess it leaves behind.
const createInFlight = new Set<string>()

/**
 * Resolves once `predicate` matches the live sidebar store, or never — the
 * same "no ceiling, self-clears the moment the real row's own reseed lands"
 * shape the pre-migration tree's own `confirmCreate` used
 * (workspace-tree-context.tsx, since deleted): a create's row always arrives
 * through the ordinary reseed/WS path everything else here already depends
 * on, so this only ever needs to notice it, never to invent a timeout for a
 * path that already has its own liveness guarantee.
 */
function waitForRow(predicate: (repos: readonly Repo[]) => boolean): Promise<void> {
  return new Promise((resolve) => {
    if (predicate(useSidebarStore.getState().repos)) {
      resolve()
      return
    }
    const unsubscribe = useSidebarStore.subscribe((state) => {
      if (!predicate(state.repos)) return
      unsubscribe()
      resolve()
    })
  })
}

/** Whether some repo's chat list now carries `chatId`, ALREADY placed under
 *  `parentId` — true only once the create that minted it has both reseeded
 *  AND its placement write has landed, never merely once its POST resolved
 *  or the chat merely exists.
 *
 *  `parentId`, and checking it, is load-bearing — this is `forkHasLanded`'s
 *  own shape, not the bare existence check this function used to be.
 *  Sidebar-placement-unification Task 8 moved a repo-scoped chat's placement
 *  onto a separate `Node` write (`CreateChat`'s own `MintChat` then
 *  `placeChat`, chats.go) — the SAME two-aggregate split Task 5 gave home
 *  rows — so `chat.parentId` no longer reliably reflects where the create
 *  actually landed the instant the chat itself is merely observed to exist:
 *  the chat lifecycle hub broadcasts on `MintChat`'s own commit alone, with
 *  no idea the placement write is still in flight, so a reseed can land here
 *  showing the chat already existing but still parented at root. See
 *  `waitForHomeChat`'s own doc, which pins the identical race for home. */
function chatHasLanded(chatId: string, parentId: string): (repos: readonly Repo[]) => boolean {
  return (repos) =>
    repos.some((r) => r.chats?.some((c) => c.id === chatId && c.parentId === parentId))
}

/** `chatHasLanded`'s own twin for a FORK: true only once BOTH halves have
 *  arrived, correctly placed — the chat (under `parentId`) AND the workspace
 *  it owns.
 *
 *  A workspace mints its owning chat chat-first (rows-from-repo.ts's own
 *  doc), so the two land as separate reseed frames, never atomically. Until
 *  the workspace frame catches up, `rows-from-repo.ts` has no WORKSPACE
 *  NODE to fold this chat onto — its render position falls through to the
 *  chat's OWN placement rules (parentId, then workspaceId-as-ground, which
 *  fails since that workspace isn't in the tree yet), landing it at the
 *  REPO ROOT rather than nested under the branch it was actually forked
 *  from. Clearing the pending spinner on existence alone revealed exactly
 *  that frame — caught live: a fresh fork appeared outside its parent for a
 *  beat, shoving every row below it down, before snapping into its real
 *  nested position the instant the workspace frame landed.
 *
 *  The chat's OWN `parentId` match is required on top of that, for the same
 *  reason `chatHasLanded` now checks it: a fork's placement is ALSO a
 *  separate Node write from its mint (`createOwnWorktreeChat` calls the same
 *  `MintChat`-then-`placeChat` sequence), so the workspace-owner half landing
 *  does not by itself guarantee the CHAT half's placement has too. */
function forkHasLanded(chatId: string, parentId: string): (repos: readonly Repo[]) => boolean {
  return (repos) =>
    repos.some(
      (r) =>
        r.chats?.some((c) => c.id === chatId && c.parentId === parentId) &&
        r.workspaces.some((w) => w.owningChatId === chatId),
    )
}

/** `waitForRow`'s own twin for a PROJECT-HOME thread: home rides no repo at
 *  all (`resolveHomeRowScope`'s own doc), so its chats live in
 *  `useHomeTreeStore`, not `useSidebarStore` — a create there is never
 *  observed by `waitForRow`'s subscription, which only ever fires on the
 *  repo-scoped store.
 *
 *  `parentId` is required, and checked — this is `forkHasLanded`'s own shape,
 *  not `chatHasLanded`'s. A home chat's placement is NOT on the `Chat`
 *  aggregate `MintChat` commits (its `ParentID` defaults to the Go zero value
 *  `""`, i.e. root) — for a home row it lives on a SEPARATE `Node` aggregate,
 *  written by a second, later call in the same backend request
 *  (`CreateChat`'s own `MintChat` then `placeChat`, chats.go). The chat
 *  lifecycle hub broadcasts on the FIRST commit alone, with no idea the
 *  second is still in flight, so a reseed can land here showing the chat
 *  already existing but still parented at root — landing this promise (and
 *  clearing the pending placeholder) on existence alone hands rendering to a
 *  REAL row that is itself still momentarily wrong. Caught live: a fresh home
 *  thread inside a folder appeared at the top of the list for a beat before
 *  snapping into the folder — the exact shape `forkHasLanded`'s own doc
 *  describes for a fork's two-aggregate mint, fixed here the same way rather
 *  than a new one invented for it. */
function waitForHomeChat(projectId: string, chatId: string, parentId: string): Promise<void> {
  const landed = (): boolean =>
    getHomeTree(projectId).chats.some((c) => c.id === chatId && c.parentId === parentId)
  return new Promise((resolve) => {
    if (landed()) {
      resolve()
      return
    }
    const unsubscribe = useHomeTreeStore.subscribe(() => {
      if (!landed()) return
      unsubscribe()
      resolve()
    })
  })
}

/** A fork create armed by `handleCreate`'s 'workspace' branch, waiting on the
 *  name the user types into the pending row's inline input before it can
 *  actually fire — keyed by that row's `tempId`. `confirmPendingCreateName`
 *  and `cancelPendingCreate` are the only two ways an entry ever leaves this
 *  map, and each releases the `createInFlight` key with it. */
const armedBranchCreates = new Map<
  string,
  {
    projectId: string
    repoId: string
    providerId: string
    placementParentId: string
    release: () => void
  }
>()

/** Creates a workspace (fork) or a thread (chat) under `parentId`. Both draw
 *  an optimistic row at the exact slot the finished create lands in
 *  (pending-creates.ts) the instant this is called — never after the round
 *  trip, which for a fork includes provisioning a real git worktree. A fork
 *  asks for its branch name first (the pending row becomes an inline input,
 *  confirmed via `confirmPendingCreateName`); a thread has nothing to name
 *  and fires immediately. */
export function handleCreate(parentId: string, kind: 'workspace' | 'thread'): void {
  // A project-home row (chat OR folder) is resolved FIRST, against every
  // visible project's home tree rather than `repos` — same rule `handleOpen`
  // already follows via the identical `resolveHomeRowScope` call. Project
  // home rides no repo at all, so there is no worktree to fork from: a
  // folder's own "+" already hides Fork for exactly this reason
  // (`rows-from-home.ts`'s `foldersCanFork: false`), and a chat row's Fork
  // button now does too (see sidebar-row.tsx's `canFork` check) — reached
  // here only via a stale click racing that, so it stays a silent no-op
  // rather than a request with nothing to act on.
  //
  // Thread, unlike Fork, is NOT refused for a folder — home applies the same
  // logic to a folder it applies to a chat: the folder names no workspace of
  // its own (home has none to name), but `homeRow.homeWorkspaceId` already IS
  // the one workspace every home row — chat or folder, nested or not — runs
  // in, so there is nothing folder-specific left to resolve below.
  const homeRow = resolveHomeRowScope(parentId)
  if (homeRow) {
    if (kind === 'workspace' || (homeRow.kind !== 'chat' && homeRow.kind !== 'folder')) return
    const provider = enabledProvider()
    if (!provider) return
    const inFlightKey = `${kind}:${parentId}`
    if (createInFlight.has(inFlightKey)) return
    createInFlight.add(inFlightKey)
    const release = (): void => {
      createInFlight.delete(inFlightKey)
    }
    // The new thread's OWN tree position, once real: nested under the
    // clicked chat's own id, after every thread already there — the same
    // rule the repo-scoped thread branch below follows.
    const siblingRows = rowsFromHome(homeRow.homeWorkspaceId, getHomeTree(homeRow.projectId).chats)
    const order = siblingRows.filter((r) => r.parentId === parentId && r.kind === 'chat').length
    const tempId = `pending-${crypto.randomUUID()}`
    usePendingCreatesStore.getState().addCreating({
      tempId,
      kind: 'chat',
      projectId: homeRow.projectId,
      parentId,
      order,
      workspaceId: homeRow.homeWorkspaceId,
      ownsWorktree: false,
    })
    // `parentId` (the THIRD arg — the clicked chat's own id) EXPLICITLY, not
    // left to default to root: home has no workspace nodes at all for the
    // fold that nests a repo-scoped thread to fall back on (see below), so
    // an omitted parentId here roots every home thread at the top level
    // regardless of which bubble was clicked — caught live: rooted as a
    // sibling of "Test", never nested under it.
    createChat(homeRow.homeWorkspaceId, provider.id, parentId)
      .then((chatId) => {
        release()
        // Hides the real row (space-scroller.tsx's `unconfirmedRealIds`)
        // from first paint, rather than letting it render wrong once and
        // correct itself a moment later — see PendingCreateEntry.realId.
        usePendingCreatesStore.getState().attachRealId(tempId, chatId)
        return waitForHomeChat(homeRow.projectId, chatId, parentId).then(() =>
          usePendingCreatesStore.getState().clear(tempId),
        )
      })
      .catch((err: unknown) => {
        release()
        usePendingCreatesStore
          .getState()
          .setError(tempId, err instanceof Error ? err.message : 'Failed to start chat')
      })
    return
  }

  const currentRepos = useSidebarStore.getState().repos
  // A bubble carries its GROUND workspace right on the chat record
  // (`Chat.workspaceId` — "its own if it owns one, otherwise the one it
  // borrows from an ancestor"), already used to open it (`openableWorkspaceOf`
  // above) — so a chat row's Fork/Thread resolve against THAT, not against
  // the clicked bubble itself. Previously this returned silently instead:
  // clicking Thread on a bubble did nothing, and Fork was offered on every
  // bubble regardless, with no target it could actually act on.
  const chatRow = resolveChatRow(currentRepos, parentId)
  // No ground at all (spec §9.2: a bubble's ancestry can resolve to nothing,
  // e.g. moved across repos) — nothing to fork or thread into, silently.
  if (chatRow && !chatRow.chat.workspaceId) return
  const found = resolveRow(currentRepos, chatRow?.chat.workspaceId ?? parentId)
  if (!found) return
  const { repo, subject } = found
  const { projectId } = repo
  if (!projectId) return

  const inFlightKey = `${kind}:${parentId}`
  if (createInFlight.has(inFlightKey)) return
  createInFlight.add(inFlightKey)
  const release = (): void => {
    createInFlight.delete(inFlightKey)
  }

  if (kind === 'workspace') {
    // Task 8: mints the workspace AND its first chat in ONE call (POST
    // .../chats {ownWorktree: true} — backend Task 7) instead of the old
    // chat-less postWorkspace, which produced a bare branch row now and a
    // separate child chat row only once something else later started a
    // conversation in it. The parent named below is the clicked row's own
    // fork parent, same as the old mapping (a folder is the one exception the
    // old `placement` distinguished — that split has no counterpart on this
    // endpoint's single `parentId`, so a folder click also just names
    // itself here) — resolved to the chat that owns it, see below.
    const provider = enabledProvider()
    if (!provider) {
      release()
      return
    }
    // The daemon places by CHAT id, and the clicked row's own id is only that
    // id for a branch row (a locked branch, the repo home — `rows-from-repo.ts`
    // draws those AS their owning `branch` chat). A REGULAR fork's row is id'd
    // from its `Workspace`, because its owner is an ordinary conversation
    // already drawn beside it, so its owning chat has to be read off the
    // workspace record instead. Falls back to the clicked row for the ids that
    // name no workspace of this repo (the repo home, a folder) and for a frame
    // that carries no owner yet.
    const owningChatId = repo.workspaces.find((w) => w.id === subject.id)?.owningChatId
    // `parentId` is only a safe fallback for a DIRECT click on the row itself
    // (its own rendered id already equals whatever this resolves to). A
    // bubble's ground workspace can ALSO be the repo home — never in
    // `repo.workspaces` for `owningChatId` to be read off — so a bubble
    // forking the home workspace needs the SAME resolution the home row's
    // own id was rendered with, not the clicked bubble's unrelated id.
    const placementParentId =
      owningChatId ||
      (subject.id === repo.defaultWorkspaceId
        ? resolveHomeOwnerId(subject.id, repo.defaultOwningChatId, repo.chats ?? [])
        : parentId)
    // The new fork's OWN tree position, once real: nested under
    // `placementParentId`, never `subject.id`. `walkTreeIntoRows` stamps a
    // REAL child row's own `parentId` with its parent's RENDERED id —
    // `node.id`, already folded onto the owning chat for any branch row that
    // resolved one (rows-from-repo.ts) — so a sibling count (and the pending
    // row's own `parentId`) keyed on `subject.id`'s raw WORKSPACE-id-space
    // value matches no real row at all whenever an owning chat exists (the
    // normal case), landing the naming/spinner row at the sidebar ROOT
    // instead of nested under the clicked branch — caught live: forking
    // "main" drew its naming input as a top-level row after every other
    // project's, not under "main" where its real fork lands.
    // `placementParentId` is exactly `handleCreate`'s own OTHER id — already
    // the rendered/folded parent id, since it is either the resolved owning
    // chat or the clicked row's own (already-rendered) id. EVERY sibling
    // counts here, any kind — folders, branches AND chats interleave on one
    // dense order (workspace-tree-utils.ts's own doc), and the backend's own
    // placement write (owning_chat.go's placeOwningRow) appends a new fork by
    // counting ALL existing rows under the same parent chat id, not just the
    // branch/folder-kind ones — a narrower count here would produce an order
    // value real siblings already hold, landing the pending row somewhere
    // other than the tail slot the real create actually appends to.
    const siblingRows = rowsFromRepo(repo)
    const order = siblingRows.filter((r) => r.parentId === placementParentId).length
    // Only one naming input is ever open at once (matching the old tree's
    // own single `creatingChildOf`) — replacing rather than stacking a
    // second one, and releasing whatever the FIRST one held (its
    // `createInFlight` lock, its `armedBranchCreates` entry) so opening a
    // second one elsewhere can never orphan the first's.
    const otherNaming = usePendingCreatesStore.getState().entries.find((e) => e.status === 'naming')
    if (otherNaming) cancelPendingCreate(otherNaming.tempId)
    const tempId = `pending-${crypto.randomUUID()}`
    armedBranchCreates.set(tempId, {
      projectId,
      repoId: repo.id,
      providerId: provider.id,
      placementParentId,
      release,
    })
    usePendingCreatesStore.getState().startNaming({
      tempId,
      kind: 'branch',
      projectId,
      parentId: placementParentId,
      order,
      workspaceId: null,
      ownsWorktree: true,
    })
    return
  }

  // The new thread's OWN tree position, once real: nested under the clicked
  // row's own chat id (`parentId`, the original argument — a thread's
  // placement lives in CHAT-id space, unlike a fork's, which lives in
  // WORKSPACE-id space above), after every thread already there. Computed
  // once, up front, and reused below for the `wsId` lookup too.
  const siblingRows = rowsFromRepo(repo)

  // A thread needs a real workspace to run in. A `workspace` subject IS one
  // (`subject.id`, not the clicked row's — this one posts to that workspace's
  // chats mount, and a branch row's own id is the chat that owns it). A
  // `folder` subject names none of its own, but a folder applies "the same
  // logic as its parent" (product rule) rather than refusing outright: its
  // nearest owning workspace is already resolved and stamped onto its own
  // `SidebarRow.workspaceId` at row-build time (`walkTreeIntoRows`'s
  // `ancestorWorkspaceId` — a locked branch, an ordinary fork, or (with no
  // ancestor branch at all) the repo's own home), so this reads that back
  // rather than re-walking the tree itself. Still null only for a subject
  // this repo's own rows never actually rendered (a stale click racing a
  // repo swap) — genuinely nothing to act on, same as before.
  const wsId =
    subject.kind === 'workspace'
      ? subject.id
      : (siblingRows.find((r) => r.id === subject.id)?.workspaceId ?? null)
  if (!wsId) {
    toast.error('Start a thread from a workspace row — a folder has none to run it in')
    release()
    return
  }
  // THE GLOBAL PROVIDER LIST, not `getOrCreateWorkspaceStore(wsId)`'s.
  //
  // Providers are machine-level — `use-workspace-agent-chats-stream.ts` says so
  // itself and mirrors every read into the global store for exactly this reason
  // — but a per-WORKSPACE store only ever holds them once that workspace has
  // been MOUNTED and run its own `seedProviders`. `getOrCreateWorkspaceStore`
  // does not mount anything: for a row the user has never opened it happily
  // mints a brand-new store whose `agentChats.providers` is `[]`, and the guard
  // below then returned with no request, no toast and nothing on screen. That
  // is the whole of "the thread button does nothing" — measured live: the
  // daemon's chat count did not move on a click. The fork branch above was
  // always right to read the global list; this one now agrees with it.
  const provider = enabledProvider()
  if (!provider) {
    release()
    return
  }
  const order = siblingRows.filter((r) => r.parentId === parentId && r.kind === 'chat').length
  const tempId = `pending-${crypto.randomUUID()}`
  usePendingCreatesStore.getState().addCreating({
    tempId,
    kind: 'chat',
    projectId,
    parentId,
    order,
    workspaceId: wsId,
    ownsWorktree: false,
  })
  // `release` fires the moment the REQUEST itself settles, not once the row
  // has visually landed: `createInFlight`'s whole job is stopping a rapid
  // double-click from firing a second POST for the same click, and gating it
  // on `waitForRow` instead would leave it stuck for as long as the reseed
  // takes — or forever, if the row's own live-update path never fires for
  // some unrelated reason. That would block every later click on this exact
  // (kind, parentId) behind a wait nothing here can bound.
  //
  // `parentId` as the THIRD arg — without it the new chat's own `parentId`
  // defaults to root, and it only LOOKED nested under the clicked row
  // whenever that row happened to also OWN `wsId` (buildSidebarTree's
  // workspace-ground fold nests every chat there under its owning row
  // regardless of its real `parentId`). Threading off any OTHER bubble
  // sharing that same workspace rooted the new chat at the top level
  // instead — caught chasing the identical gap on the project-home path,
  // which has no workspace-ground fold to hide it behind at all.
  createChat(wsId, provider.id, parentId)
    .then((chatId) => {
      release()
      announceTreeChange(repo.id)
      // Hides the real row (space-scroller.tsx's `unconfirmedRealIds`) from
      // first paint — see PendingCreateEntry.realId, and chatHasLanded's own
      // doc for the placement race this closes for repo-scoped threads too.
      usePendingCreatesStore.getState().attachRealId(tempId, chatId)
      return waitForRow(chatHasLanded(chatId, parentId)).then(() =>
        usePendingCreatesStore.getState().clear(tempId),
      )
    })
    .catch((err: unknown) => {
      release()
      usePendingCreatesStore
        .getState()
        .setError(tempId, err instanceof Error ? err.message : 'Failed to start chat')
    })
}

/**
 * Confirms a fork's pending row — the inline input's Enter/blur — with the
 * typed branch name: flips the row to its spinner state and fires the
 * create `handleCreate`'s 'workspace' branch armed but did not send. Absent
 * from `armedBranchCreates` means the row already left naming (a stale
 * confirm racing a cancel elsewhere) — a no-op rather than a second request.
 */
export function confirmPendingCreateName(tempId: string, name: string): void {
  const armed = armedBranchCreates.get(tempId)
  if (!armed) return
  armedBranchCreates.delete(tempId)
  usePendingCreatesStore.getState().confirmNaming(tempId, name)
  // `armed.release` fires the moment the REQUEST itself settles — see the
  // identical reasoning on the thread path above; the same hang risk applies
  // here, and a stuck naming lock would leave every later "+" click on this
  // exact row permanently inert.
  createChatWithOwnWorktree(armed.projectId, armed.repoId, armed.providerId, armed.placementParentId, name)
    .then((chatId) => {
      armed.release()
      announceTreeChange(armed.repoId)
      // Hides the real row (space-scroller.tsx's `unconfirmedRealIds`) from
      // first paint — see PendingCreateEntry.realId, and forkHasLanded's own
      // doc for the placement race this closes.
      usePendingCreatesStore.getState().attachRealId(tempId, chatId)
      return waitForRow(forkHasLanded(chatId, armed.placementParentId)).then(() =>
        usePendingCreatesStore.getState().clear(tempId),
      )
    })
    .catch((err: unknown) => {
      armed.release()
      usePendingCreatesStore
        .getState()
        .setError(tempId, err instanceof Error ? err.message : 'Failed to create workspace')
    })
}

/** Drops a pending row outright: a naming input the user cancelled (Escape,
 *  or blurred empty — never reached the network, nothing to roll back) or an
 *  error the user dismissed. Releasing `createInFlight` here, not in
 *  `handleCreate`, is what keeps a second "+" click on the same row inert
 *  for as long as its naming input is still open. */
export function cancelPendingCreate(tempId: string): void {
  const armed = armedBranchCreates.get(tempId)
  if (armed) {
    armedBranchCreates.delete(tempId)
    armed.release()
  }
  usePendingCreatesStore.getState().clear(tempId)
}

/**
 * The sidebar header's "start a thread on the project's home workspace"
 * button — NOT reachable through `handleCreate` above, which resolves its
 * `parentId` against the repo-scoped sidebar store (`resolveRow`) and has no
 * notion of project home at all (home-workspace-resolver.ts: "home is a
 * project-level concept, not a repo workspace" — it never appears in
 * `repos`). Creates directly against the resolved home workspace id, then
 * opens it the same way `navigateThenOpenChat` opens a freshly-forked repo
 * chat: navigate to project home, wait for it to become the active
 * workspace, then open the chat in its own pane. `homeWorkspaceId` is the
 * caller's job to resolve (home-workspace-resolver.ts's
 * `useHomeWorkspaceState`/`ensureHomeWorkspaceResolved`) — this function
 * only spends it.
 */
export async function handleCreateHomeThread(
  projectId: string,
  homeWorkspaceId: string,
  navigate: NavigateFn,
): Promise<void> {
  const provider = enabledProvider()
  if (!provider) return
  let chatId: string
  try {
    chatId = await createChat(homeWorkspaceId, provider.id)
  } catch (err) {
    toast.error(err instanceof Error ? err.message : 'Failed to start chat')
    return
  }
  await openHomeChat(projectId, homeWorkspaceId, chatId, navigate)
}

/**
 * Open `chatId` (already existing, running in the project's home workspace)
 * the way a click does — the home-scoped sibling of `navigateThenOpenChat`.
 * Shared by {@link handleCreateHomeThread} (a freshly-minted chat) and
 * {@link handleOpen}'s home branch (an existing row the user clicked): both
 * need the identical sequence — open in place if home is already the active
 * workspace, otherwise navigate to project home and wait for it to actually
 * become active before opening, exactly as `navigateThenOpenChat` does for a
 * repo chat. `/ide/$projectId/home` carries no `repoId`/`wsId` of its own
 * (project home rides no repo), which is the one thing that keeps this from
 * just being a call to `navigateThenOpenChat` itself.
 */
async function openHomeChat(
  projectId: string,
  homeWorkspaceId: string,
  chatId: string,
  navigate: NavigateFn,
): Promise<void> {
  if (openChatInOwnView(chatId, homeWorkspaceId)) return
  await navigate({ to: '/ide/$projectId/home', params: { projectId } })
  const becameActive = await waitForActiveWorkspace(homeWorkspaceId)
  if (!becameActive) return
  openChatInOwnPane(paneOpenSubject(chatId, homeWorkspaceId))
}

/**
 * The provider a new chat is started with, or null — having SAID SO — when
 * there is none.
 *
 * Both create paths used to return silently here. A silent return is
 * indistinguishable from a dead button, and it is the shape both halves of "the
 * fork and thread buttons do nothing" took: one because it genuinely had no
 * providers to find (see the thread branch's own note), the other because a
 * real outage empties this list (`use-workspace-agent-chats-stream.ts` toasts
 * once for that, but only for a MOUNTED workspace — the sidebar can be the only
 * thing on screen). A precondition that stops a click has to be visible.
 */
export function enabledProvider(): { id: string } | null {
  const provider = useAgentProvidersStore.getState().providers.find((p) => p.enabled)
  if (provider) return provider
  toast.error(
    'No agent provider is enabled',
    'Enable one in Settings → Providers to start a chat or fork a workspace.',
  )
  return null
}

/**
 * Tell `repoId`'s sidebar tree to re-read its rows after this client created
 * one.
 *
 * See `removal-commit.ts`'s `bumpRepoTree` for the full story — same signal,
 * same reason, the create half. The daemon really does mint the chat and its
 * worktree (measured live: the repo's chat count went 9 -> 11 on two clicks),
 * but `openRepoTreeSubscription` reseeds `crowbar_chats` only on this
 * generation moving, and the only thing that normally moves it is a chat frame
 * arriving for a MOUNTED workspace of this repo. Fork from the sidebar with no
 * workspace of that repo open — on the project-home route, say — and the row
 * never appeared at all. The button had worked; nothing had drawn it.
 */
function announceTreeChange(repoId: string): void {
  if (repoId) useFolderSignalStore.getState().bump(repoId)
}
