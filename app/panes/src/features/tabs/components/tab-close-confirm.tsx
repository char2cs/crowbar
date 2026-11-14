import { useStore } from 'zustand'
import { saveBuffer } from '@/features/editor/lib/buffer-save'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { useBufferActions } from '@/features/workspace/stores/hooks/use-buffer-store'
import UnsavedChangesDialog from '@/features/window/components/unsaved-changes-dialog'

/** Asks what to do with the edits of a dirty tab being closed in `paneId`. Every tab strip
 *  mounts one; only the pane the close came from answers. */
export function TabCloseConfirm({ paneId }: { paneId?: string }) {
  const pendingClose = useStore(windowPaneStore, (s) =>
    s.pendingClose?.paneId === paneId ? s.pendingClose : null,
  )
  const fileName = useStore(
    windowPaneStore,
    (s) => s.buffers.find((b) => b.id === s.pendingClose?.bufferId)?.name ?? '',
  )
  const { confirmPendingClose, setPendingClose } = useBufferActions()
  if (!pendingClose) return null

  return (
    <UnsavedChangesDialog
      fileName={fileName}
      // Save the tab being closed (not whichever is active); a failed save keeps it open.
      onSave={async () => {
        if (await saveBuffer(pendingClose.bufferId)) confirmPendingClose()
      }}
      onDiscard={confirmPendingClose}
      onCancel={() => setPendingClose(null)}
    />
  )
}
