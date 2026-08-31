import { editorAPI } from '@/features/editor/extensions/api'
import { useJumpListStore, type JumpListEntry } from '@/features/editor/stores/jump-list-store'
import { useEditorStateStore } from '@/features/editor/stores/state-store'
import { useEditorUIStore } from '@/features/editor/stores/ui-store'
import { readWorkspaceFile } from '@/features/file-system/controllers/platform'
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

  /**
   * Make `bufferId` visible, guaranteeing the pane can actually render it.
   *
   * A pane renders `pane.bufferIds`-derived buffers only, resolving the active
   * one as `paneBuffers.find((b) => b.id === pane.activeBufferId) ?? null` (see
   * pane-container.tsx). `activatePaneBuffer` sets `activeBufferId` but never
   * touches `bufferIds`, so activating a buffer the pane does not hold leaves it
   * pointing at nothing and the editor renders BLANK.
   *
   * History navigation hits that routinely: the file's tab may have been closed
   * while its buffer stayed alive, or the buffer may live in another pane. So
   * switch to the pane that actually holds it, and only attach to the active
   * pane when no pane does.
   *
   * Mirrors the reveal-don't-duplicate rule `openContent` already applies to
   * terminal and agent-chat buffers for this same blank-pane reason.
   */
  const reveal = (bufferId: string): void => {
    // The Back/Forward handshake names the buffer id RECORDED in the entry, but
    // the buffer actually shown can differ: a closed file is reopened under a
    // brand-new id, and a file re-opened by the user since is found by path. The
    // recorder only ever sees the id that lands here, so point the marker at it
    // — otherwise the recorder records this jump as a new navigation, which
    // truncates the forward branch and breaks Back/Forward from then on.
    if (bufferId !== entry.bufferId) jumpActions().retargetNavigation(bufferId)
    const holdingPane = paneStore.paneActions.getPaneByEditorTabId(bufferId)
    const paneId = holdingPane?.id ?? paneStore.activePaneId
    if (holdingPane) {
      paneStore.paneActions.activateEditorTabInPane(paneId, bufferId)
    } else {
      // addEditorTabToPane always activates the tab it adds, so no separate
      // activate call is needed on this branch.
      const buf = paneStore.buffers.find((b) => b.id === bufferId)
      paneStore.paneActions.addEditorTabToPane(paneId, {
        id: bufferId,
        type: 'editor',
        name: buf?.name ?? bufferId,
        workspaceId: buf?.workspaceId ?? wsStore.workspaceId,
      })
    }
  }

  // Hide completions and reset input timestamp to prevent completions from triggering
  const uiActions = useEditorUIStore.getState().actions
  uiActions.setIsLspCompletionVisible(false)
  uiActions.setLastInputTimestamp(0)

  // Try to find the buffer by ID first, then by path. Task 26: buffers are
  // window-level — scope the path lookup to THIS workspace (a buffer id is
  // already globally unique, so the id lookup needs no such filter).
  let targetBuffer = paneStore.buffers.find((b) => b.id === entry.bufferId)

  if (!targetBuffer) {
    targetBuffer = paneStore.buffers.find(
      (b) => b.path === entry.filePath && b.workspaceId === wsStore.workspaceId,
    )
  }

  if (!targetBuffer) {
    // Buffer is closed, try to reopen the file.
    //
    // Read through the workspace this entry belongs to, never the "active"
    // workspace at call time: for linked worktrees of one repo the same relative
    // path exists in every checkout, so resolving late can load a sibling
    // worktree's content into the buffer (see readWorkspaceFile's contract).
    // The workspace guard at the top of this function is what makes
    // `wsStore.workspaceId` the entry's OWN workspace and not merely today's.
    try {
      const content = await readWorkspaceFile(wsStore.workspaceId, entry.filePath)
      const fileName = entry.filePath.split('/').pop() || 'untitled'
      const bufferId = paneStore.bufferActions.openContent({
        type: 'editor',
        path: entry.filePath,
        name: fileName,
        content,
        workspaceId: wsStore.workspaceId,
      })
      reveal(bufferId)
    } catch (error) {
      logger.error('JumpList', 'Failed to reopen file:', entry.filePath, error)
      return abandon()
    }
  } else {
    reveal(targetBuffer.id)
  }

  // Set cursor position and scroll after buffer is ready
  setTimeout(() => {
    editorAPI.setCursorPosition({
      line: entry.line,
      column: entry.column,
      offset: entry.offset,
    })

    useEditorStateStore.getState().actions.setScroll(entry.scrollTop, entry.scrollLeft)

    logger.info('JumpList', `Jumped to ${entry.filePath}:${entry.line}:${entry.column}`)
  }, 100)

  return true
}
