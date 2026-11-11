import { readSelect, selectRepos, selectProjects } from '@/core/entities/select'
import { treeUiStore, type RemovalDraft } from '@/features/workspace/tree/tree-ui-store'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { planRemoval, type DragSubject } from './removal-plan'
import { resolveHomeRowScope } from '@/core/entities/home-rows'
import { resolveChatRow, resolveRow } from './open-actions'

/** Closes the panes showing a chat that was just held, so the window is already elsewhere
 *  when the delete lands instead of being pulled out of it. */
function leaveHeldChats(drafts: readonly RemovalDraft[]): void {
  const held = new Set(drafts.flatMap((draft) => draft.hiddenIds))
  const { panes, paneActions } = windowPaneStore.getState()
  for (const [paneId, pane] of Object.entries(panes)) {
    if (pane.chatId && held.has(pane.chatId)) paneActions.closePane(paneId)
  }
}

// The one trash path for every row kind (row trash and drag-to-trash), held in the removal tray
// for undo, against the store's raw repos. False when nothing was held: an unknown id, the repo
// home (trash the repo instead), or a locked workspace the daemon would refuse.
export function handleTrash(id: string, drainMs?: number): boolean {
  const currentRepos = readSelect(selectRepos)
  const homeRow = resolveHomeRowScope(id)
  // Home rows first: the repo walk can falsely match a home folder, which the daemon's folder
  // list leaks into every repo stamped with that repo's id.
  const chatRow = homeRow ? null : resolveChatRow(currentRepos, id)
  const subject: DragSubject | null = homeRow
    ? { kind: homeRow.kind, id }
    : chatRow
      ? { kind: 'chat', id, repoId: chatRow.repo.id }
      : (resolveRow(currentRepos, id)?.subject ?? null)
  if (!subject) return false
  const projects = readSelect(selectProjects)
  const drafts = planRemoval([subject], currentRepos, projects)
  if (drafts.length === 0) return false
  treeUiStore.getState().hold(drafts, drainMs)
  leaveHeldChats(drafts)
  return true
}

// Trashes a whole PROJECT through the same tray (§9); `RemovalTray` confirms the cascading kinds
// before commit, so no dialog up front. False for an id no loaded project claims.
export function handleTrashProject(projectId: string): boolean {
  const currentRepos = readSelect(selectRepos)
  const projects = readSelect(selectProjects)
  const drafts = planRemoval([{ kind: 'project', id: projectId }], currentRepos, projects)
  if (drafts.length === 0) return false
  treeUiStore.getState().hold(drafts)
  return true
}

// Trashes a whole REPO, which `handleTrash` cannot: the repo-home row resolves to its default
// branch, and trashing that would delete one branch out from under the repo.
export function handleTrashRepo(repoId: string): boolean {
  const currentRepos = readSelect(selectRepos)
  const projects = readSelect(selectProjects)
  const drafts = planRemoval([{ kind: 'repo', id: repoId }], currentRepos, projects)
  if (drafts.length === 0) return false
  treeUiStore.getState().hold(drafts)
  return true
}
