import { useStore } from 'zustand'
import { saveBuffer } from '@/features/editor/lib/buffer-save'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { useBufferActions } from '@/features/workspace/stores/hooks/use-buffer-store'
import UnsavedChangesDialog from '@/features/window/components/unsaved-changes-dialog'

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
      onSave={async () => {
        if (await saveBuffer(pendingClose.bufferId)) confirmPendingClose()
      }}
      onDiscard={confirmPendingClose}
      onCancel={() => setPendingClose(null)}
    />
  )
}
