import type { useNavigate } from '@tanstack/react-router'
import { useSidebarStore, type Chat, type Repo } from '@/lib/store/sidebar'
import type { DragSubject } from './removal-plan'
import { getActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'
import { workspaceIdOfBranchRow } from '@/components/sidebar/lib/branch-row-id'
import { toast } from '@/features/window/stores/toast-store'
import { openChatInOwnPane } from '@/components/sidebar/lib/drop-actions'
import { resolveHomeRowScope } from '@/lib/store/home-tree'
import { rowsFromRepo } from '@/components/sidebar/lib/rows-from-repo'
import { chatNotLoadedYet } from '@/components/sidebar/lib/row-actions'
import type { SidebarRow as SidebarRowType } from '@/components/sidebar/types/sidebar-row'

// Opening a sidebar row: resolving what an id names, and putting its chat in
// a pane (navigating to its workspace or project home first when needed).

export type NavigateFn = ReturnType<typeof useNavigate>

/** What `id` resolves to: its owning repo, and the subject a drag/removal call needs. */
export interface ResolvedRow {
  repo: Repo
  subject: DragSubject
}

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
 * Validates a resolved workspace id against THIS repo, or null when it names
 * none. Spec §9.2 makes a repo's chats an open set (a bubble moved across
 * repos keeps ancestors in the repo it left), so a `wsId` naming a workspace
 * outside this repo is an ordinary state, not corruption — and routing to
 * /ide/:p/:r/:ws with a ws that is not under :r would be a URL nothing
 * resolves.
 *
 * Takes an already-resolved id rather than a `Chat` — see `handleOpen`'s own
 * call site for WHERE that id comes from. It used to be `chat.workspaceId`
 * alone, which is genuinely null for a true bubble (§3.1: it owns none), and
 * that was read as "this row opens nothing" — clicking one just toggled its
 * own (childless, so invisible) fold. Live-reported: "clicking on a not
 * opened row... simply anything happens... it should create a view on its
 * own." A bubble still opens SOMEWHERE, though: it always sits inside some
 * real workspace's tree (worst case, the repo's own home), and that ground is
 * exactly what `rows-from-repo.ts`'s `ancestorWorkspaceId` already resolves
 * for it — the caller reads the row's own `workspaceId` (which now folds
 * that fallback in) rather than the chat's, and this function's only job is
 * left as the repo-membership check.
 */
function isOpenableWorkspaceOfRepo(repo: Repo, wsId: string | null): string | null {
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
export function openChatInOwnView(chatId: string, workspaceId: string): boolean {
  if (workspaceId !== getActiveWorkspaceId()) return false
  openChatInOwnPane(paneOpenSubject(chatId, workspaceId))
  return true
}

/**
 * Navigate to `wsId`'s own route, then open `chatId` into its own view.
 *
 * This is the sequencing `openChatInOwnView` alone cannot do: a bare
 * `navigate()` only changes the URL, and clicking a workspace that was not
 * already active used to stop there — the click looked like it did nothing,
 * because nothing ever wrote a chat into a pane.
 *
 * Opens the pane RIGHT AFTER navigating, not once the workspace "becomes
 * active" (an earlier version of this function polled
 * `getActiveWorkspaceId()` for up to 2s and gave up silently if it never
 * matched) — that wait can never resolve while the active PANE already holds
 * a chat from a DIFFERENT workspace: `IDEShell`'s own
 * `effectiveActiveWorkspaceId` resolves the active PANE's workspace before
 * ever consulting the route (`activePaneWorkspaceId ?? activeWorkspaceId`,
 * ide-shell.tsx), by design, for the unrelated case of clicking between two
 * panes of an existing split. That priority does not move just because THIS
 * navigation changed the URL, so "wait for active" deadlocked forever on
 * anything but the very first chat opened in a session — live-reported as a
 * bubble chat's click doing nothing at all. Panes are window-level now (Task
 * 26): `openChatInOwnPane` writes straight into `windowPaneStore`, and
 * `paneOpenSubject`'s own `workspaceId` is the HINT `resolveChatWorkspaceId`
 * needs to resolve correctly before any workspace store for it even exists
 * (see that function's own doc) — nothing here ever depended on "active"
 * being true, only on believing it had to.
 */
export async function navigateThenOpenChat(
  navigate: NavigateFn,
  params: { projectId: string; repoId: string; wsId: string },
  chatId: string,
): Promise<void> {
  await navigate({ to: '/ide/$projectId/$repoId/$wsId', params })
  openChatInOwnPane(paneOpenSubject(chatId, params.wsId))
}

/**
 * Resolve `chatId`'s route — project home or a repo workspace — and open it
 * into its own pane (reveal if already up, fill a vacant pane, or mint a new
 * view), navigating first when its workspace is not already active. Returns
 * whether a route was found at all.
 *
 * THE one place that resolves "home or repo, then open" for an
 * already-identified chat id — `handleOpen`'s three call sites below all go
 * through it now instead of each repeating the same
 * home-check/`openChatInOwnView`/`navigateThenOpenChat` trio. That
 * duplication is what let `focusRecent` (recents-actions.ts, the Recents
 * band's click) drift out of sync with this file: it never learned project
 * home exists, so a Recents row for a project-home chat resolved nowhere and
 * did nothing on click. `focusRecent` now calls this same function.
 */
export function openChatRoute(
  repos: readonly Repo[],
  chatId: string,
  workspaceId: string,
  navigate: NavigateFn,
): boolean {
  const homeRow = resolveHomeRowScope(chatId)
  if (homeRow) {
    void openHomeChat(homeRow.projectId, homeRow.homeWorkspaceId, chatId, navigate)
    return true
  }
  const found = resolveRow(repos, workspaceId)
  if (!found?.repo.projectId) return false
  if (openChatInOwnView(chatId, workspaceId)) return true
  void navigateThenOpenChat(
    navigate,
    { projectId: found.repo.projectId, repoId: found.repo.id, wsId: workspaceId },
    chatId,
  )
  return true
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
    openChatRoute(repos, id, homeRow.homeWorkspaceId, navigate)
    return
  }

  const chatRow = resolveChatRow(repos, id)
  if (chatRow) {
    // The row's OWN `workspaceId`, not the raw `Chat.workspaceId` — a bubble
    // with none of its own already has this filled in with its nearest real
    // ancestor workspace (`rows-from-repo.ts`'s own `ancestorWorkspaceId`
    // fallback), the ground it has always conceptually belonged to. Rebuilt
    // here rather than threaded in from a caller: `resolveChatRow` above
    // works off the raw store, and this is the one place that needs the
    // rendered row too — same "build this repo's rows, read the matching
    // one" shape `handleCreate`'s own Fork/Thread resolution already uses.
    const rowWsId = rowsFromRepo(chatRow.repo).find((r) => r.id === id)?.workspaceId ?? null
    const wsId = isOpenableWorkspaceOfRepo(chatRow.repo, rowWsId)
    if (!wsId || !chatRow.repo.projectId) {
      useSidebarStore.getState().toggleChatRow(id)
      return
    }
    openChatRoute(repos, id, wsId, navigate)
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
  // `subject.id`, never the row's — a branch row is addressed by its owning
  // chat, and only `resolveRow` knows which workspace that names.
  if (owningChatId) {
    openChatRoute(repos, owningChatId, found.subject.id, navigate)
    return
  }
  // No owner recorded yet: the daemon mints one on the first read of the
  // workspace list, so this is a list still landing, never a row to navigate
  // to — a bare workspace route would spin forever on a chat-keyed explorer.
  const branch = found.repo.workspaces.find((w) => w.id === found.subject.id)?.branch
  toast.error(chatNotLoadedYet('open', branch))
}

/**
 * Open `chatId` (already existing, running in the project's home workspace)
 * the way a click does — the home-scoped sibling of `navigateThenOpenChat`.
 * Shared by {@link handleCreateHomeThread} (a freshly-minted chat) and
 * {@link handleOpen}'s home branch (an existing row the user clicked): both
 * need the identical sequence — open in place if home is already the active
 * workspace, otherwise navigate to project home and open the pane right
 * after, exactly as `navigateThenOpenChat` does for a repo chat (see that
 * function's own doc for why this no longer waits for "active" first).
 * `/ide/$projectId/home` carries no `repoId`/`wsId` of its own (project home
 * rides no repo), which is the one thing that keeps this from just being a
 * call to `navigateThenOpenChat` itself.
 */
export async function openHomeChat(
  projectId: string,
  homeWorkspaceId: string,
  chatId: string,
  navigate: NavigateFn,
): Promise<void> {
  if (openChatInOwnView(chatId, homeWorkspaceId)) return
  await navigate({ to: '/ide/$projectId/home', params: { projectId } })
  openChatInOwnPane(paneOpenSubject(chatId, homeWorkspaceId))
}
