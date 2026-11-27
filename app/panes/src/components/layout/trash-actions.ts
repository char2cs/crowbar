import { readSelect, selectRepos, selectProjects } from '@/core/entities/select'
import { treeUiStore, type RemovalDraft } from '@/features/workspace/tree/tree-ui-store'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { planRemoval, type DragSubject } from './removal-plan'
import { resolveHomeRowScope } from '@/core/entities/home-rows'
import { resolveChatRow, resolveRow } from './open-actions'

function leaveHeldChats(drafts: readonly RemovalDraft[]): void {
  const held = new Set(drafts.flatMap((draft) => draft.hiddenIds))
  const { panes, paneActions } = windowPaneStore.getState()
  for (const [paneId, pane] of Object.entries(panes)) {
    if (pane.chatId && held.has(pane.chatId)) paneActions.closePane(paneId)
  }
}

export function handleTrash(id: string, drainMs?: number): boolean {
  const currentRepos = readSelect(selectRepos)
  const homeRow = resolveHomeRowScope(id)
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

export function handleTrashRepo(repoId: string): boolean {
  const currentRepos = readSelect(selectRepos)
  const projects = readSelect(selectProjects)
  const drafts = planRemoval([{ kind: 'repo', id: repoId }], currentRepos, projects)
  if (drafts.length === 0) return false
  treeUiStore.getState().hold(drafts)
  return true
}
