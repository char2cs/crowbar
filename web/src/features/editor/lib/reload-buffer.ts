import { readDocument } from '@/lib/api/documents'
import { documentRefOf } from '@/features/editor/documents/document-ref'
import { applyExternalText, documentText } from '@/features/editor/documents/document-text'
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
  const opened = findEditorBuffer(bufferId)
  if (!opened || opened.isVirtual || opened.path.startsWith('untitled:')) return 'failed'
  const ref = documentRefOf(opened)
  const seen = documentText(ref)
  const buffer = findEditorBuffer(bufferId)
  if (seen === null || !buffer) return 'failed'
  if (buffer.isDirty && !confirmDiscard(buffer)) return 'kept'
  cancelAutosave(bufferId)

  let disk: string
  try {
    disk = (await readDocument(ref)).text
  } catch {
    return 'failed'
  }

  if (documentText(ref) !== seen) return 'kept'
  applyExternalText(ref, disk, { saved: true })
  return 'reloaded'
}
