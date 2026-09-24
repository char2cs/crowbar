import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { isEditorContent } from '@/features/panes/types/pane-content'
import { syncBufferWithDisk } from '@/features/workspace/lib/external-buffer-sync'
import { toast } from '@/features/window/stores/toast-store'

/**
 * A tab's Reload (P0-9): re-read the file from disk into the same buffer.
 *
 * It used to close the tab and reopen it 100ms later from the in-memory
 * buffer, marking whatever was in memory as saved — unsaved edits silently
 * became "clean" and were lost on the next external change. Now it reads the
 * disk, and a buffer with unsaved edits is refused rather than clobbered.
 */
export function reloadTabFromDisk(bufferId: string): void {
  const buf = windowPaneStore.getState().buffers.find((b) => b.id === bufferId)
  if (!buf || !isEditorContent(buf) || buf.isVirtual || !buf.path) return
  if (buf.isDirty) {
    toast.warning(
      `${buf.name} has unsaved changes`,
      'Save or undo them first — reloading would discard them.',
    )
    return
  }
  void syncBufferWithDisk(buf.workspaceId, buf.path)
}
