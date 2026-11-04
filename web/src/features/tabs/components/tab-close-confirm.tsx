import { useStore } from 'zustand'
import { saveBuffer } from '@/features/editor/lib/buffer-save'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { useBufferActions } from '@/features/workspace/stores/hooks/use-buffer-store'
import { usePaneActions } from '@/features/workspace/stores/hooks/use-pane-store'
import UnsavedChangesDialog from '@/features/window/components/unsaved-changes-dialog'

/** Asks what to do with the edits of a dirty tab being closed in `paneId`. */
export function TabCloseConfirm({ paneId }: { paneId?: string }) {
  const pendingClose = useStore(windowPaneStore, (s) => s.pendingClose)
  const fileName = useStore(
    windowPaneStore,
    (s) => s.buffers.find((b) => b.id === s.pendingClose?.bufferId)?.name ?? '',
  )
  const { removeEditorTabFromPane } = usePaneActions()
  const { confirmPendingClose, setPendingClose } = useBufferActions()
  if (!pendingClose) return null

  const close = () => {
    if (paneId) removeEditorTabFromPane(paneId, pendingClose.bufferId)
    confirmPendingClose()
  }
  return (
    <UnsavedChangesDialog
      fileName={fileName}
      // Save the tab being closed (not whichever is active); a failed save keeps it open.
      onSave={async () => {
        if (await saveBuffer(pendingClose.bufferId)) close()
      }}
      onDiscard={close}
      onCancel={() => setPendingClose(null)}
    />
  )
}
