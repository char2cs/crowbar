import { useSidebarStore } from '@/lib/store/sidebar'
import { useRemovalTrayStore } from '@/lib/store/sidebar-removal'
import { useProjectDataStore, EMPTY_PROJECTS } from '@/lib/store/projects'
import { dataOf } from '@/lib/loadable'
import { planRemoval, type DragSubject } from './removal-plan'
import { resolveHomeRowScope } from '@/lib/store/home-tree'
import { resolveChatRow, resolveRow } from './open-actions'

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
 * `RemovalTray` pops `RemovalConfirmDialog` for the two cascading kinds
 * (`repo`, `project`) before it commits, so no dialog is needed up front.
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
