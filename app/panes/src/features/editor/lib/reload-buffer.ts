import { readDocument } from '@/lib/api/documents'
import { documentRefOf, documentUri, isUntitledRef } from '@/features/editor/documents/document-ref'
import { applyExternalText, documentText } from '@/features/editor/documents/document-text'
import { awaitsRetry, retryLoad } from '@/features/editor/documents/open-document'
import { cancelPendingWrites, findEditorBuffer, type PathedBuffer } from './buffer-save'

export type ReloadOutcome = 'reloaded' | 'kept' | 'failed'

export async function reloadBufferFromDisk(
  bufferId: string,
  confirmDiscard: (buffer: PathedBuffer) => boolean,
): Promise<ReloadOutcome> {
  const opened = findEditorBuffer(bufferId)
  if (!opened || opened.isVirtual || isUntitledRef(opened)) return 'failed'
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
