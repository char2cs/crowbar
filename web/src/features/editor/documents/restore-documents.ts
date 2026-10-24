import { isNotFoundError } from '@/lib/api'
import { readDocument } from '@/lib/api/documents'
import { getEditorHost } from '@/features/editor/host/editor-host'
import { setDocumentDirty } from '@/features/editor/lib/buffer-save'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { isEditorContent, type EditorContent } from '@/features/panes/types/pane-content'
import { toast } from '@/features/window/stores/toast-store'
import {
  flagExternalChange,
  setBufferFileMissing,
} from '@/features/workspace/lib/external-buffer-sync'
import { documentRefOf, documentUri } from './document-ref'
import { clearDraft, loadDrafts, type EditorDraft } from './draft-store'
import { documentModelOpener, documentRefusal, type OpenDocumentModel } from './open-document'

type Refusals = Map<string, string>

const unsaved = (draft: EditorDraft | undefined): draft is EditorDraft =>
  !!draft && draft.text !== draft.savedText

const stillOpen = (buffer: EditorContent) =>
  windowPaneStore.getState().buffers.some((b) => b.id === buffer.id)

/** Opens one restored buffer's document: its draft over disk, disk alone, or nothing it can show. */
async function restoreDocument(
  buffer: EditorContent,
  draft: EditorDraft | undefined,
  openModel: OpenDocumentModel,
  refusals: Refusals,
): Promise<void> {
  const ref = documentRefOf(buffer)
  if (buffer.isVirtual) {
    openModel(ref, draft?.text ?? '', draft?.savedText ?? '')
    return settle(buffer, draft)
  }
  let disk: string
  try {
    disk = (await readDocument(ref)).text
  } catch (err) {
    const refusal = documentRefusal(err)
    if (refusal) refusals.set(refusal.title, refusal.message)
    if (!stillOpen(buffer)) return
    // The draft is the only copy of its edits, so it opens whatever disk says.
    if (unsaved(draft)) openModel(ref, draft.text, draft.savedText)
    else if (isNotFoundError(err)) setBufferFileMissing(ref, true)
    return settle(buffer, draft)
  }
  if (!stillOpen(buffer)) return
  if (buffer.fileMissing) setBufferFileMissing(ref, false)
  if (!unsaved(draft)) {
    openModel(ref, disk)
    return settle(buffer, draft)
  }
  openModel(ref, draft.text, draft.savedText)
  settle(buffer, draft)
  if (disk !== draft.savedText) flagExternalChange(ref)
}

/** The buffer mirrors its document's dirty flag; a draft it no longer needs goes. */
function settle(buffer: EditorContent, draft: EditorDraft | undefined): void {
  const uri = documentUri(documentRefOf(buffer))
  const documents = getEditorHost()?.documents
  if (!documents?.has(uri)) return
  const dirty = documents.isDirty(uri)
  if (dirty || draft || buffer.isDirty) setDocumentDirty(uri, dirty)
}

/**
 * Opens the document of every editor buffer the saved layout brought back.
 * A dirty draft wins over disk and is flagged when disk moved under it; a
 * refusal keeps the tab and says why once; drafts of closed buffers go.
 */
export async function restoreDocuments(): Promise<void> {
  const [openModel, drafts] = await Promise.all([documentModelOpener(), loadDrafts()])
  const refusals: Refusals = new Map()
  const buffers = windowPaneStore
    .getState()
    .buffers.filter((b): b is EditorContent => isEditorContent(b) && !!b.path)
  const held = new Set<string>()
  const restores: Promise<void>[] = []
  for (const buffer of buffers) {
    const uri = documentUri(documentRefOf(buffer))
    held.add(uri)
    if (getEditorHost()?.documents.has(uri)) continue
    restores.push(restoreDocument(buffer, drafts.get(uri), openModel, refusals))
  }
  await Promise.allSettled(restores)
  for (const uri of drafts.keys()) if (!held.has(uri)) clearDraft(uri)
  for (const [title, message] of refusals) toast.error(title, message)
}
