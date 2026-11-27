import { isNotFoundError } from '@/lib/errors/api-error'
import { readDocument, type DocumentRef } from '@/lib/api/documents'
import { readSelect, selectRepos } from '@/core/entities/select'
import { workspaceNames } from '@/lib/workspace/workspace-branch'
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

const unsaved = (draft: EditorDraft | undefined, disk?: string | null): draft is EditorDraft =>
  !!draft && draft.text !== draft.savedText && draft.text !== disk

const stillOpen = (buffer: EditorContent) =>
  windowPaneStore.getState().buffers.some((b) => b.id === buffer.id)

interface RestoreWatch {
  open: OpenDocumentModel
  refused: { refusal: unknown } | null
  end(): void
}

async function watchFor(
  buffer: EditorContent,
  openModel: OpenDocumentModel,
): Promise<RestoreWatch> {
  const ref = documentRefOf(buffer)
  const unwatched = { open: openModel, refused: null, end: () => {} }
  if (ref.authority !== 'host' || getEditorHost()?.documents.has(documentUri(ref))) {
    return unwatched
  }
  let unwatch: (() => void) | null = null
  let refused: RestoreWatch['refused'] = null
  try {
    unwatch = await followHostDocument(ref)
  } catch (refusal) {
    refused = { refusal }
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

async function loadDocument(
  buffer: EditorContent,
  draft: EditorDraft | undefined,
  openModel: OpenDocumentModel,
  watch: RestoreWatch,
  failures: Failures,
): Promise<void> {
  const ref = documentRefOf(buffer)
  const since = documentSaveMark(ref)
  const read: Read = buffer.isVirtual
    ? { disk: '', ref }
    : await readDocument(ref).then(
        (doc) => ({ disk: doc.text, ref: doc.ref }),
        (error: unknown) => ({ error }),
      )
  if (!stillOpen(buffer)) return
  if ('disk' in read) setDocumentUnavailable(ref, null)
  if (getEditorHost()?.documents.has(documentUri(ref))) {
    return meetOpenDocument(buffer, draft, openModel, read, since)
  }
  if ('error' in read) {
    const reason = openFailure(ref, read.error)
    if (!isNotFoundError(read.error)) failures.push(failure(ref, reason))
    if (!unsaved(draft)) return setDocumentUnavailable(ref, reason)
    setDocumentUnavailable(ref, null)
  }
  const disk = 'disk' in read ? read.disk : null
  if (watch.refused) {
    if (!unsaved(draft, disk)) {
      const reason = watchRefusal(ref, read, watch.refused.refusal)
      failures.push(failure(ref, reason))
      return setDocumentUnavailable(ref, reason)
    }
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
  if (findDocumentBuffer(ref)?.unavailable && !isDocumentDirty(ref)) {
    setDocumentUnavailable(ref, openFailure(ref, read.error))
  }
}

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
  reopenDocument(uri, () => openModel(ref, draft.text, draft.savedText))
  settle(buffer, draft)
  if (disk !== draft.savedText) flagExternalChange(ref)
}

function settle(buffer: EditorContent, draft: EditorDraft | undefined): void {
  const uri = documentUri(documentRefOf(buffer))
  const documents = getEditorHost()?.documents
  if (!documents?.has(uri)) return
  const dirty = documents.isDirty(uri)
  if (dirty || draft || buffer.isDirty) setDocumentDirty(uri, dirty)
}

const tail = (path: string, n: number) => path.split('/').slice(-n).join('/')

function fileLabel({ message, path }: Failure, kind: Failures): string {
  if (!`/${path}`.endsWith(`/${message}`)) return message
  const clashes = (n: number) =>
    kind.some((f) => f.message === message && f.path !== path && tail(f.path, n) === tail(path, n))
  let n = 1
  while (n < path.split('/').length && clashes(n)) n++
  return tail(path, n)
}

function failureLabel(failure: Failure, kind: Failures, names: Map<string, string>): string {
  const label = fileLabel(failure, kind)
  const twin = kind.some((f) => f.path === failure.path && f.workspaceId !== failure.workspaceId)
  return twin ? `${label} (${names.get(failure.workspaceId) ?? failure.workspaceId})` : label
}

function sayFailures(failures: Failures): void {
  const kinds = new Map<string, Failures>()
  for (const failure of failures) {
    kinds.set(failure.title, [...(kinds.get(failure.title) ?? []), failure])
  }
  for (const [title, kind] of kinds) {
    const ids = [...new Set(kind.map((f) => f.workspaceId))]
    const names = workspaceNames(readSelect(selectRepos), ids)
    const labels = new Set(kind.map((failure) => failureLabel(failure, kind, names)))
    toast.error(title, [...labels].join(', '))
  }
}

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

export async function restoreDocuments(): Promise<void> {
  const buffers = windowPaneStore
    .getState()
    .buffers.filter((b): b is EditorContent => isEditorContent(b) && !!b.path)
  const held = new Set(buffers.map((b) => documentUri(documentRefOf(b))))
  let loaded: [OpenDocumentModel | null, Map<string, EditorDraft>]
  try {
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
