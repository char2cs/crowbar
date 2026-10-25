import { readDocument } from '@/lib/api/documents'
import { documentRefOf, documentUri } from '@/features/editor/documents/document-ref'
import { applyExternalText, documentText } from '@/features/editor/documents/document-text'
import { awaitsRetry, retryLoad } from '@/features/editor/documents/open-document'
import { cancelPendingWrites, findEditorBuffer, type PathedBuffer } from './buffer-save'

export type ReloadOutcome = 'reloaded' | 'kept' | 'failed'

/**
 * Replace a buffer's text with what is on disk, as one undoable edit. A dirty buffer
 * reloads only if `confirmDiscard` agrees; edits typed during the read abandon it.
 */
export async function reloadBufferFromDisk(
  bufferId: string,
  confirmDiscard: (buffer: PathedBuffer) => boolean,
): Promise<ReloadOutcome> {
  const opened = findEditorBuffer(bufferId)
  if (!opened || opened.isVirtual || opened.path.startsWith('untitled:')) return 'failed'
  // A tab whose document failed to load reads it again, as a reopen does.
  if (awaitsRetry(opened)) return (await retryLoad(opened)) ? 'reloaded' : 'failed'
  const ref = documentRefOf(opened)
  const seen = documentText(ref)
  const buffer = findEditorBuffer(bufferId)
  if (seen === null || !buffer) return 'failed'
  if (buffer.isDirty && !confirmDiscard(buffer)) return 'kept'
  cancelPendingWrites(documentUri(ref))

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
