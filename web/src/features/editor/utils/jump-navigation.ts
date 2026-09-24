import { revealInEditor } from '@/features/editor/lib/reveal'
import { useJumpListStore, type JumpListEntry } from '@/features/editor/stores/jump-list-store'
import { getActiveWorkspaceStoreRef } from '@/features/workspace/stores/workspace-store-ref'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { logger } from './logger'

export async function navigateToJumpEntry(entry: JumpListEntry): Promise<boolean> {
  const jumpActions = () => useJumpListStore.getState().actions

  /** Navigation that did not happen must not leave the "this activation came
   *  from Back/Forward" marker armed: it would suppress the next genuine visit
   *  to that buffer id, long after the failed jump is forgotten. */
  const abandon = (): false => {
    jumpActions().clearNavigationTarget()
    return false
  }

  const wsStore = getActiveWorkspaceStoreRef()?.getState()
  if (!wsStore) return abandon()
  // Task 26: panes/buffers are window-level now — `wsStore` only still owns
  // `workspaceId` (and other per-workspace slices); pane/buffer state and
  // actions come from the one window store.
  const paneStore = windowPaneStore.getState()

  // The jump list survives workspace switches but its paths are workspace
  // RELATIVE, so an entry recorded elsewhere cannot be resolved here: sibling
  // worktrees of one repo hold the same `src/app.ts` with different content,
  // and following it would silently show the wrong file under the right tab
  // title. Refuse rather than guess. An unstamped entry names no workspace to
  // disagree with, so it is still honoured (graceful fallback, no migration).
  if (entry.workspaceId && entry.workspaceId !== wsStore.workspaceId) {
    logger.info(
      'JumpList',
      `Skipped ${entry.filePath}: recorded in workspace ${entry.workspaceId}, active is ${wsStore.workspaceId}`,
    )
    return abandon()
  }

  // The Back/Forward handshake names the buffer id RECORDED in the entry, but
  // the buffer actually shown can differ: a closed file is reopened under a
  // brand-new id, and a file re-opened by the user since is found by path.
  // Point the marker at the id that is shown — otherwise the recorder records
  // this jump as a new navigation, which truncates the forward branch.
  const path = paneStore.buffers.find((b) => b.id === entry.bufferId)?.path ?? entry.filePath
  try {
    const shown = await revealInEditor({
      workspaceId: wsStore.workspaceId,
      path,
      position: { line: entry.line, character: entry.column },
      scroll: { top: entry.scrollTop, left: entry.scrollLeft },
      beforeShow: (bufferId) => {
        if (bufferId !== entry.bufferId) jumpActions().retargetNavigation(bufferId)
      },
    })
    if (!shown) return abandon()
  } catch (error) {
    logger.error('JumpList', 'Failed to reopen file:', entry.filePath, error)
    return abandon()
  }
  logger.info('JumpList', `Jumped to ${entry.filePath}:${entry.line}:${entry.column}`)
  return true
}
