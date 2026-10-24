import { isNotFoundError } from '@/lib/api'
import { readDocument } from '@/lib/api/documents'
import { getEditorHost } from '@/features/editor/host/editor-host'
import { setDocumentDirty } from '@/features/editor/lib/buffer-save'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { isEditorContent, type EditorContent } from '@/features/panes/types/pane-content'
import { toast } from '@/features/window/stores/toast-store'
import { flagExternalChange } from '@/features/workspace/lib/external-buffer-sync'
import { documentRefOf, documentUri } from './document-ref'
import { clearDraft, loadDrafts, type EditorDraft } from './draft-store'
import {
  documentModelOpener,
  openFailure,
  setDocumentUnavailable,
  type OpenDocumentModel,
} from './open-document'

type Failures = Map<string, string>

/** The draft holds edits that are on neither side: not its base text, and not `disk`. */
const unsaved = (draft: EditorDraft | undefined, disk?: string | null): draft is EditorDraft =>
  !!draft && draft.text !== draft.savedText && draft.text !== disk

const stillOpen = (buffer: EditorContent) =>
  windowPaneStore.getState().buffers.some((b) => b.id === buffer.id)

/** Opens one restored buffer's document: its draft over disk, disk alone, or why it cannot. */
async function restoreDocument(
  buffer: EditorContent,
  draft: EditorDraft | undefined,
  openModel: OpenDocumentModel,
  failures: Failures,
): Promise<void> {
  const ref = documentRefOf(buffer)
  // An untitled document has no file: its draft is all there is.
  const read = buffer.isVirtual
    ? { disk: '' }
    : await readDocument(ref).then(
        (doc) => ({ disk: doc.text }),
        (error: unknown) => ({ error }),
      )
  if (!stillOpen(buffer)) return
  // Decided after the read: a reconcile or a reopen may have opened the document meanwhile.
  if (getEditorHost()?.documents.has(documentUri(ref))) return adoptDraft(buffer, draft, openModel)
  if ('error' in read) {
    const reason = openFailure(ref, read.error)
    // A missing file is said on its tab alone.
    if (!isNotFoundError(read.error)) failures.set(reason.title, reason.message)
    // A draft still opens, being the only copy of its edits.
    if (!unsaved(draft)) return setDocumentUnavailable(ref, reason)
  }
  const disk = 'disk' in read ? read.disk : null
  setDocumentUnavailable(ref, null)
  if (!unsaved(draft, disk)) {
    openModel(ref, disk ?? '')
    return settle(buffer, draft)
  }
  openModel(ref, draft.text, draft.savedText)
  settle(buffer, draft)
  if (disk !== null && disk !== draft.savedText) flagExternalChange(ref)
}

/** Another opener read disk first: an unsaved draft takes that model's place while it is untouched. */
function adoptDraft(
  buffer: EditorContent,
  draft: EditorDraft | undefined,
  openModel: OpenDocumentModel,
): void {
  const ref = documentRefOf(buffer)
  const uri = documentUri(ref)
  const documents = getEditorHost()?.documents
  const disk = documents?.savedText(uri)
  if (!unsaved(draft, disk) || documents?.isDirty(uri)) return settle(buffer, draft)
  // Reopened rather than edited, so a restored draft never arms an autosave.
  documents?.release(uri)
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
 * failed read keeps the tab and says why; drafts of closed buffers go.
 */
export async function restoreDocuments(): Promise<void> {
  const buffers = windowPaneStore
    .getState()
    .buffers.filter((b): b is EditorContent => isEditorContent(b) && !!b.path)
  const held = new Set(buffers.map((b) => documentUri(documentRefOf(b))))
  // Monaco loads only for a layout that holds an editor tab.
  const [openModel, drafts] = await Promise.all([
    buffers.length > 0 ? documentModelOpener() : null,
    loadDrafts(),
  ])
  for (const uri of drafts.keys()) if (!held.has(uri)) clearDraft(uri)
  if (!openModel) return
  const failures: Failures = new Map()
  const restores = buffers.map((buffer) => {
    const uri = documentUri(documentRefOf(buffer))
    const draft = drafts.get(uri)
    if (getEditorHost()?.documents.has(uri)) return adoptDraft(buffer, draft, openModel)
    return restoreDocument(buffer, draft, openModel, failures)
  })
  await Promise.allSettled(restores)
  for (const [title, message] of failures) toast.error(title, message)
}
