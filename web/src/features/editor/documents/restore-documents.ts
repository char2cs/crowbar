import { isNotFoundError } from '@/lib/api'
import { readDocument, type DocumentRef } from '@/lib/api/documents'
import { useSidebarStore } from '@/lib/store/sidebar'
import {
  getEditorHost,
  releaseWithDocument,
  reopenDocument,
} from '@/features/editor/host/editor-host'
import { setDocumentDirty } from '@/features/editor/lib/buffer-save'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import {
  isEditorContent,
  type DocumentUnavailable,
  type EditorContent,
} from '@/features/panes/types/pane-content'
import { toast } from '@/features/window/stores/toast-store'
import {
  flagExternalChange,
  followHostDocument,
  takeDiskText,
} from '@/features/workspace/lib/external-buffer-sync'
import { documentSaveMark, documentText, isDocumentDirty } from './document-text'
import { documentName, documentRefOf, documentUri } from './document-ref'
import { clearDraft, loadDraft, loadDrafts, type EditorDraft } from './draft-store'
import {
  documentModelOpener,
  findDocumentBuffer,
  openFailure,
  setDocumentUnavailable,
  watchRefusal,
  type OpenDocumentModel,
} from './open-document'

type Failure = DocumentUnavailable & { path: string; workspaceId: string }
type Failures = Failure[]
type Read = { disk: string; ref: DocumentRef } | { error: unknown }

const failure = (ref: DocumentRef, reason: DocumentUnavailable): Failure => ({
  ...reason,
  path: ref.path,
  workspaceId: ref.authority === 'workspace' ? ref.workspaceId : '',
})

/** The draft holds edits that are on neither side: not its base text, and not `disk`. */
const unsaved = (draft: EditorDraft | undefined, disk?: string | null): draft is EditorDraft =>
  !!draft && draft.text !== draft.savedText && draft.text !== disk

const stillOpen = (buffer: EditorContent) =>
  windowPaneStore.getState().buffers.some((b) => b.id === buffer.id)

/** A host document's watch through one restore: the model it opens holds it, or it goes. */
interface RestoreWatch {
  /** Opens a model, which holds the watch from then on. */
  open: OpenDocumentModel
  /** The daemon refused the watch: only an unsaved draft opens, and unwatched. */
  refused: boolean
  end(): void
}

async function watchFor(
  buffer: EditorContent,
  openModel: OpenDocumentModel,
): Promise<RestoreWatch> {
  const ref = documentRefOf(buffer)
  const unwatched = { open: openModel, refused: false, end: () => {} }
  // A workspace file is its files stream's; an open model already holds its own watch.
  if (ref.authority !== 'host' || getEditorHost()?.documents.has(documentUri(ref))) {
    return unwatched
  }
  // Each restored host tab holds its own socket, as an opened one does.
  let unwatch: (() => void) | null = null
  let refused = false
  try {
    unwatch = await followHostDocument(ref)
  } catch {
    // The refusal carries no reason; the read that follows tells it.
    refused = true
  }
  return {
    refused,
    open: (opened, text, savedText) => {
      openModel(opened, text, savedText)
      if (unwatch) releaseWithDocument(documentUri(opened), unwatch)
      unwatch = null
    },
    end: () => {
      unwatch?.()
      unwatch = null
    },
  }
}

/** Opens one restored buffer's document, a host one armed before it is read. */
async function restoreDocument(
  buffer: EditorContent,
  draft: EditorDraft | undefined,
  openModel: OpenDocumentModel,
  failures: Failures,
): Promise<void> {
  const watch = await watchFor(buffer, openModel)
  try {
    await loadDocument(buffer, draft, openModel, watch, failures)
  } finally {
    watch.end()
  }
}

/** Its draft over disk, disk alone, or why it cannot. */
async function loadDocument(
  buffer: EditorContent,
  draft: EditorDraft | undefined,
  openModel: OpenDocumentModel,
  watch: RestoreWatch,
  failures: Failures,
): Promise<void> {
  const ref = documentRefOf(buffer)
  const since = documentSaveMark(ref)
  // An untitled document has no file: its draft is all there is.
  const read: Read = buffer.isVirtual
    ? { disk: '', ref }
    : await readDocument(ref).then(
        (doc) => ({ disk: doc.text, ref: doc.ref }),
        (error: unknown) => ({ error }),
      )
  if (!stillOpen(buffer)) return
  // A read that worked answers why the tab could not load, whoever holds the model.
  if ('disk' in read) setDocumentUnavailable(ref, null)
  // Decided after the read: a reconcile or a reopen may have opened the document meanwhile.
  if (getEditorHost()?.documents.has(documentUri(ref))) {
    return meetOpenDocument(buffer, draft, openModel, read, since)
  }
  if ('error' in read) {
    const reason = openFailure(ref, read.error)
    // A missing file is said on its tab alone.
    if (!isNotFoundError(read.error)) failures.push(failure(ref, reason))
    // A draft still opens, being the only copy of its edits.
    if (!unsaved(draft)) return setDocumentUnavailable(ref, reason)
    setDocumentUnavailable(ref, null)
  }
  const disk = 'disk' in read ? read.disk : null
  if (watch.refused) {
    if (!unsaved(draft, disk)) {
      const reason = watchRefusal(ref, read)
      failures.push(failure(ref, reason))
      return setDocumentUnavailable(ref, reason)
    }
    // The draft opens unwatched; the next reopen or reconcile follows it again.
    if (!('error' in read) || isNotFoundError(read.error)) {
      failures.push(
        failure(ref, { title: 'Not following changes on disk', message: documentName(ref) }),
      )
    }
  }
  if (!unsaved(draft, disk)) {
    watch.open(ref, disk ?? '')
    return settle(buffer, draft)
  }
  watch.open(ref, draft.text, draft.savedText)
  settle(buffer, draft)
  if (disk !== null && disk !== draft.savedText) flagExternalChange(ref)
}

/** The document opened before or during the read: a draft may take its place, and the read lands if it is not older. */
function meetOpenDocument(
  buffer: EditorContent,
  draft: EditorDraft | undefined,
  openModel: OpenDocumentModel,
  read: Read,
  since: number,
): void {
  const ref = documentRefOf(buffer)
  adoptDraft(buffer, draft, openModel)
  if ('disk' in read) {
    if (!buffer.isVirtual) takeDiskText(ref, read.disk, since)
    return
  }
  // A later failure replaces the reason a clean document's tab still shows.
  if (findDocumentBuffer(ref)?.unavailable && !isDocumentDirty(ref)) {
    setDocumentUnavailable(ref, openFailure(ref, read.error))
  }
}

/** Restores one tab; whatever fails, a tab left without its document says why rather than blank. */
async function failSafe(
  buffer: EditorContent,
  failures: Failures,
  draft: EditorDraft | undefined,
  restore: () => Promise<void> | void,
): Promise<void> {
  try {
    await restore()
  } catch (error) {
    const ref = documentRefOf(buffer)
    if (!stillOpen(buffer)) return
    if (getEditorHost()?.documents.has(documentUri(ref))) {
      // The document shows; what failed after it opened is logged, and said if it cost the draft.
      console.error('Restoring a document failed:', error)
      if (unsaved(draft, documentText(ref))) {
        failures.push(
          failure(ref, { title: 'Unsaved changes not restored', message: documentName(ref) }),
        )
      }
      return
    }
    const reason = openFailure(ref, error)
    failures.push(failure(ref, reason))
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
  reopenDocument(uri, () => openModel(ref, draft.text, draft.savedText))
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

/** A workspace as the user knows it: its branch, else its id. */
function workspaceName(workspaceId: string): string {
  for (const repo of useSidebarStore.getState().repos) {
    const ws = repo.workspaces.find((w) => w.id === workspaceId)
    if (ws) return ws.branch
  }
  return workspaceId
}

/** A message naming its file, by as much of its path as tells it from the others named alike. */
function fileLabel({ message, path }: Failure, kind: Failures): string {
  if (!`/${path}`.endsWith(`/${message}`)) return message
  const clashes = (n: number) =>
    kind.some((f) => f.message === message && f.path !== path && tail(f.path, n) === tail(path, n))
  let n = 1
  while (n < path.split('/').length && clashes(n)) n++
  return tail(path, n)
}

/** The same file failing in two worktrees is told apart by its workspace. */
function failureLabel(failure: Failure, kind: Failures): string {
  const label = fileLabel(failure, kind)
  const twin = kind.some((f) => f.path === failure.path && f.workspaceId !== failure.workspaceId)
  return twin ? `${label} (${workspaceName(failure.workspaceId)})` : label
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
    return failure(ref, reason)
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
    return failSafe(buffer, failures, draft, () =>
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
    await failSafe(buffer, [], draft, () => restoreDocument(buffer, draft, openModel, []))
  } catch (error) {
    setDocumentUnavailable(ref, openFailure(ref, error))
  }
  const now = windowPaneStore.getState().buffers.find((b) => b.id === buffer.id)
  return (now && isEditorContent(now) && now.unavailable) || null
}
