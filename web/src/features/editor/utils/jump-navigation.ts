import { revealInEditor } from '@/features/editor/lib/reveal'
import { useJumpListStore, type JumpListEntry } from '@/features/editor/stores/jump-list-store'
import { documentRefOf } from '@/features/editor/documents/document-ref'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { isEditorContent } from '@/features/panes/types/pane-content'
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

  // The Back/Forward handshake names the buffer id RECORDED in the entry, but
  // the buffer actually shown can differ: a closed file is reopened under a
  // brand-new id, and a file re-opened by the user since is found by its
  // document. Point the marker at the id that is shown — otherwise the
  // recorder records this jump as a new navigation, which truncates the
  // forward branch.
  const recorded = windowPaneStore.getState().buffers.find((b) => b.id === entry.bufferId)
  const doc = recorded && isEditorContent(recorded) ? documentRefOf(recorded) : entry.doc
  try {
    const shown = await revealInEditor({
      doc,
      position: { line: entry.line, character: entry.column },
      beforeShow: (bufferId) => {
        if (bufferId !== entry.bufferId) jumpActions().retargetNavigation(bufferId)
      },
    })
    if (!shown) return abandon()
  } catch (error) {
    logger.error('JumpList', 'Failed to reopen file:', entry.doc.path, error)
    return abandon()
  }
  logger.info('JumpList', `Jumped to ${entry.doc.path}:${entry.line}:${entry.column}`)
  return true
}
