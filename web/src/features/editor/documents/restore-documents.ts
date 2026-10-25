import { isNotFoundError } from '@/lib/api'
import { readDocument, type DocumentRef } from '@/lib/api/documents'
import { getEditorHost } from '@/features/editor/host/editor-host'
import { setDocumentDirty } from '@/features/editor/lib/buffer-save'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import {
  isEditorContent,
  type DocumentUnavailable,
  type EditorContent,
} from '@/features/panes/types/pane-content'
import { toast } from '@/features/window/stores/toast-store'
import { flagExternalChange } from '@/features/workspace/lib/external-buffer-sync'
import { applyExternalText, savedDocumentText } from './document-text'
import { documentRefOf, documentUri } from './document-ref'
import { clearDraft, loadDraft, loadDrafts, type EditorDraft } from './draft-store'
import {
  documentModelOpener,
  openFailure,
  setDocumentUnavailable,
  type OpenDocumentModel,
} from './open-document'

type Failures = (DocumentUnavailable & { path: string })[]

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
  // A read that worked answers why the tab could not load, whoever holds the model.
  if ('disk' in read) setDocumentUnavailable(ref, null)
  // Decided after the read: a reconcile or a reopen may have opened the document meanwhile.
  if (getEditorHost()?.documents.has(documentUri(ref))) {
    adoptDraft(buffer, draft, openModel)
    if ('disk' in read && !buffer.isVirtual) catchUp(ref, read.disk)
    return
  }
  if ('error' in read) {
    const reason = openFailure(ref, read.error)
    // A missing file is said on its tab alone.
    if (!isNotFoundError(read.error)) failures.push({ ...reason, path: ref.path })
    // A draft still opens, being the only copy of its edits.
    if (!unsaved(draft)) return setDocumentUnavailable(ref, reason)
    setDocumentUnavailable(ref, null)
  }
  const disk = 'disk' in read ? read.disk : null
  if (!unsaved(draft, disk)) {
    openModel(ref, disk ?? '')
    return settle(buffer, draft)
  }
  openModel(ref, draft.text, draft.savedText)
  settle(buffer, draft)
  if (disk !== null && disk !== draft.savedText) flagExternalChange(ref)
}

/** An open document meets the disk text just read: a clean one takes it, a dirty one is flagged. */
function catchUp(ref: DocumentRef, disk: string): void {
  if (disk === savedDocumentText(ref)) return
  if (getEditorHost()?.documents.isDirty(documentUri(ref))) return flagExternalChange(ref)
  applyExternalText(ref, disk, { saved: true })
}

/** Restores one tab; whatever fails, a tab left without its document says why rather than blank. */
async function failSafe(
  buffer: EditorContent,
  failures: Failures,
  restore: () => Promise<void> | void,
): Promise<void> {
  try {
    await restore()
  } catch (error) {
    const ref = documentRefOf(buffer)
    if (!stillOpen(buffer) || getEditorHost()?.documents.has(documentUri(ref))) return
    const reason = openFailure(ref, error)
    failures.push({ ...reason, path: ref.path })
    setDocumentUnavailable(ref, reason)
  }
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

const tail = (path: string, n: number) => path.split('/').slice(-n).join('/')

/** A message naming its file, by as much of its path as tells it from the others named alike. */
function failureLabel({ message, path }: Failures[number], kind: Failures): string {
  if (!`/${path}`.endsWith(`/${message}`)) return message
  const clashes = (n: number) =>
    kind.some((f) => f.message === message && f.path !== path && tail(f.path, n) === tail(path, n))
  let n = 1
  while (n < path.split('/').length && clashes(n)) n++
  return tail(path, n)
}

/** One toast per kind of failure, naming every file it hit. */
function sayFailures(failures: Failures): void {
  const kinds = new Map<string, Failures>()
  for (const failure of failures) {
    kinds.set(failure.title, [...(kinds.get(failure.title) ?? []), failure])
  }
  for (const [title, kind] of kinds) {
    const labels = new Set(kind.map((failure) => failureLabel(failure, kind)))
    toast.error(title, [...labels].join(', '))
  }
}

/** Nothing could load (Monaco or the drafts): each tab says so, and retries as after a failed read. */
function failAll(buffers: EditorContent[], error: unknown): void {
  const failed = buffers.filter(
    (b) => stillOpen(b) && !getEditorHost()?.documents.has(documentUri(documentRefOf(b))),
  )
  const failures: Failures = failed.map((b) => {
    const ref = documentRefOf(b)
    const reason = openFailure(ref, error)
    setDocumentUnavailable(ref, reason)
    return { ...reason, path: ref.path }
  })
  sayFailures(failures)
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
  let loaded: [OpenDocumentModel | null, Map<string, EditorDraft>]
  try {
    // Monaco loads only for a layout that holds an editor tab.
    loaded = await Promise.all([buffers.length > 0 ? documentModelOpener() : null, loadDrafts()])
  } catch (error) {
    return failAll(buffers, error)
  }
  const [openModel, drafts] = loaded
  for (const uri of drafts.keys()) if (!held.has(uri)) clearDraft(uri)
  if (!openModel) return
  const failures: Failures = []
  const restores = buffers.map((buffer) => {
    const uri = documentUri(documentRefOf(buffer))
    const draft = drafts.get(uri)
    return failSafe(buffer, failures, () =>
      getEditorHost()?.documents.has(uri)
        ? adoptDraft(buffer, draft, openModel)
        : restoreDocument(buffer, draft, openModel, failures),
    )
  })
  await Promise.all(restores)
  sayFailures(failures)
}

/**
 * Loads a tab whose document failed to load as the boot restore would, its draft over
 * disk. Answers why it still cannot, or null once the document is open.
 */
export async function retryDocument(buffer: EditorContent): Promise<DocumentUnavailable | null> {
  const ref = documentRefOf(buffer)
  try {
    const [openModel, draft] = await Promise.all([
      documentModelOpener(),
      loadDraft(documentUri(ref)),
    ])
    await failSafe(buffer, [], () => restoreDocument(buffer, draft, openModel, []))
  } catch (error) {
    setDocumentUnavailable(ref, openFailure(ref, error))
  }
  const now = windowPaneStore.getState().buffers.find((b) => b.id === buffer.id)
  return (now && isEditorContent(now) && now.unavailable) || null
}
