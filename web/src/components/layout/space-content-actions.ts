import type { useNavigate } from '@tanstack/react-router'
import { useSidebarStore, type Chat, type Repo } from '@/lib/store/sidebar'
import { useRemovalTrayStore } from '@/lib/store/sidebar-removal'
import { useProjectDataStore, EMPTY_PROJECTS } from '@/lib/store/projects'
import { dataOf } from '@/lib/loadable'
import { planRemoval, type DragSubject } from './removal-plan'
import { createChat, createChatWithOwnWorktree } from '@/features/agent/api/agent-api'
import {
  getActiveWorkspaceId,
  getOrCreateWorkspaceStore,
} from '@/features/workspace/stores/workspace-store-registry'
import { useAgentProvidersStore } from '@/features/settings/stores/agent-providers-store'
import { workspaceIdOfBranchRow } from '@/components/sidebar/lib/branch-row-id'
import { toast } from '@/features/window/stores/toast-store'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { openChatIntoPane } from '@/components/sidebar/lib/drop-actions'
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
 * the chat id space: `rows-from-repo.ts` gives a locked branch and a repo home
 * the id of the chat that owns their workspace, because that is what the daemon
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
  for (const repo of repos) {
    // A `branch` row lives in the chat id space but is NOT a chat row: it is
    // how a locked branch or a repo home is drawn, and `rows-from-repo.ts`
    // gives that workspace's row this id precisely so the daemon can place
    // under it. Matching it here would send every verb down the chat path —
    // "+" would go silently inert, trash would delete the branch's own row,
    // and rename would retitle it instead of moving the git branch.
    const chat = repo.chats?.find((c) => c.id === id && c.type !== 'branch')
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
 *  workspace for `openChatIntoPane`, which reads only those two fields
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
 * Open `chatId` (which runs in `workspaceId`) the way a click does (spec
 * §8.4) — into the active pane, reusing the exact drop mechanics
 * `openChatIntoPane` already gives a dragged chat (dedup-reveal if it is
 * already up, plain open into an empty pane, merge/split otherwise; never a
 * silent swap — see its own doc).
 *
 * Only reachable when `workspaceId` IS ALREADY the active workspace:
 * `openChatIntoPane` itself refuses otherwise (its own documented guard — no
 * chatId->workspace resolution exists yet on the render side for an
 * off-screen workspace). A row naming a workspace that is not yet active goes
 * through `navigateThenOpenChat` below instead, which waits for it to become
 * active first rather than racing it.
 */
function openChatInActivePane(chatId: string, workspaceId: string): boolean {
  if (workspaceId !== getActiveWorkspaceId()) return false
  const { activePaneId } = windowPaneStore.getState()
  openChatIntoPane(paneOpenSubject(chatId, workspaceId), activePaneId, 'center')
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
 * Navigate to `wsId`'s own route, then open `chatId` into the active pane
 * once the workspace has actually finished becoming active.
 *
 * This is the sequencing `openChatInActivePane` alone cannot do: a bare
 * `navigate()` only changes the URL, and clicking a workspace that was not
 * already active used to stop there — the click looked like it did nothing,
 * because nothing ever wrote a chat into a pane. `openChatIntoPane`'s own
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
  const { activePaneId } = windowPaneStore.getState()
  openChatIntoPane(paneOpenSubject(chatId, params.wsId), activePaneId, 'center')
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
 */
export function handleOpen(id: string, repos: readonly Repo[], navigate: NavigateFn): void {
  const chatRow = resolveChatRow(repos, id)
  if (chatRow) {
    const wsId = openableWorkspaceOf(chatRow.repo, chatRow.chat)
    const projectId = chatRow.repo.projectId
    if (!wsId || !projectId) {
      useSidebarStore.getState().toggleChatRow(id)
      return
    }
    if (openChatInActivePane(id, wsId)) return
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
  if (owningChatId && openChatInActivePane(owningChatId, found.subject.id)) return
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
 * and a user-locked, non-home workspace (`planRemoval`'s `draftFor` refuses
 * one — the daemon would refuse the delete too, so the tray must never
 * accept one and promise otherwise).
 */
export function handleTrash(id: string): boolean {
  const currentRepos = useSidebarStore.getState().repos
  // Checked BEFORE resolveRow, which cannot see a chat at all
  // (`resolveChatRow`'s own doc: "callers must consult THIS FIRST").
  const chatRow = resolveChatRow(currentRepos, id)
  const subject: DragSubject | null = chatRow
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

/** Creates a workspace (fork) or a thread (chat) under `parentId`. */
export function handleCreate(parentId: string, kind: 'workspace' | 'thread'): void {
  const currentRepos = useSidebarStore.getState().repos
  // A chat row's "+" is inert for now, and it must be SILENTLY inert: falling
  // through to resolveRow's miss is what it did before chat rows existed, but
  // now that `resolveChatRow` can see one, the thread branch below would reach
  // its "a folder has none to run it in" refusal — a sentence that is simply
  // untrue of a chat, which is precisely the row a thread hangs off. Wiring it
  // needs the cwd walk (§3.2) to find a bubble's ground workspace; until then
  // an affordance that does nothing beats one that explains itself wrongly.
  if (resolveChatRow(currentRepos, parentId)) return
  const found = resolveRow(currentRepos, parentId)
  if (!found) return
  const { repo, subject } = found
  const { projectId } = repo
  if (!projectId) return

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
    //
    // A generated branch name is no longer minted client-side either: the
    // server names it the same way Promote's spontaneous create already
    // does (model spec §4.1), since this is the same "nothing of its own
    // to name the branch" shape.
    const provider = useAgentProvidersStore.getState().providers.find((p) => p.enabled)
    if (!provider) return
    // The daemon places by CHAT id, and the clicked row's own id is only that
    // id for a branch row (a locked branch, the repo home — `rows-from-repo.ts`
    // draws those AS their owning `branch` chat). A REGULAR fork's row is id'd
    // from its `Workspace`, because its owner is an ordinary conversation
    // already drawn beside it, so its owning chat has to be read off the
    // workspace record instead. Falls back to the clicked row for the ids that
    // name no workspace of this repo (the repo home, a folder) and for a frame
    // that carries no owner yet.
    const owningChatId = repo.workspaces.find((w) => w.id === subject.id)?.owningChatId
    createChatWithOwnWorktree(projectId, repo.id, provider.id, owningChatId || parentId).catch(
      (err: unknown) => {
        toast.error(err instanceof Error ? err.message : 'Failed to create workspace')
      },
    )
    return
  }

  // A thread needs a real workspace to run in — a folder names none. Reachable
  // only from an empty folder's affordance dropdown (its own "+" always makes
  // a workspace, see above); the dropdown itself has no folder-vs-workspace
  // split to hide this option behind, so say why instead of swallowing the
  // click.
  if (subject.kind !== 'workspace') {
    toast.error('Start a thread from a workspace row — a folder has none to run it in')
    return
  }
  // `subject.id`, not the clicked row's — this one needs the WORKSPACE (it
  // opens that workspace's store and posts to its chats mount), and a branch
  // row's own id is the chat that owns it.
  const wsId = subject.id
  const store = getOrCreateWorkspaceStore(wsId)
  const provider = store.getState().agentChats.providers.find((p) => p.enabled)
  if (!provider) return
  createChat(wsId, provider.id).catch((err: unknown) => {
    toast.error(err instanceof Error ? err.message : 'Failed to start chat')
  })
}
