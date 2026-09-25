import { readWorkspaceFile } from '@/features/file-system/controllers/platform'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { isEditorContent } from '@/features/panes/types/pane-content'
import { cancelAutosave, findEditorBuffer, type PathedBuffer } from './buffer-save'

export type ReloadOutcome = 'reloaded' | 'kept' | 'failed'

/**
 * Replace a buffer's content with what is on disk. A dirty buffer is only
 * reloaded when `confirmDiscard` agrees; edits typed while the read is in
 * flight are never overwritten (the reload is abandoned instead).
 *
 * The open Monaco model follows through the normal store → model sync, which
 * applies it as an undoable edit.
 */
export async function reloadBufferFromDisk(
  bufferId: string,
  confirmDiscard: (buffer: PathedBuffer) => boolean,
): Promise<ReloadOutcome> {
  if (typeof window !== 'undefined') window.dispatchEvent(new Event('flush-editor-content'))
  const buffer = findEditorBuffer(bufferId)
  if (!buffer || buffer.isVirtual || buffer.path.startsWith('untitled:')) return 'failed'
  if (buffer.isDirty && !confirmDiscard(buffer)) return 'kept'
  cancelAutosave(bufferId)

  const seen = buffer.content
  let disk: string
  try {
    disk = await readWorkspaceFile(buffer.workspaceId, buffer.path)
  } catch {
    return 'failed'
  }

  const current = findEditorBuffer(bufferId)
  if (!current || current.content !== seen) return 'kept'
  windowPaneStore.setState((state) => ({
    buffers: state.buffers.map((b) =>
      b.id === bufferId && isEditorContent(b)
        ? {
            ...b,
            content: disk,
            savedContent: disk,
            isDirty: false,
            hasExternalChange: false,
            fileMissing: false,
          }
        : b,
    ),
  }))
  return 'reloaded'
}
