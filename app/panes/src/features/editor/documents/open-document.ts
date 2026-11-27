import { ApiError } from '@/lib/errors/api-error'
import {
  DAEMON_STATE_DOCUMENT_CODE,
  HOST_DOCUMENT_TRANSPORT_CODE,
  isWatchLimit,
  readDocument,
  workspaceRef,
  type DocumentRead,
  type DocumentRef,
  type DocumentTarget,
} from '@/lib/api/documents'
import { isTauri } from '@/lib/crowbar-bridge'
import { openNativeDialog } from '@/lib/native-dialog'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { homeOf } from '@/features/panes/lib/view-state'
import {
  isEditorContent,
  type DocumentUnavailable,
  type EditorContent,
} from '@/features/panes/types/pane-content'
import { toast } from '@/features/window/stores/toast-store'
import { hostFailureRefusal, toastHostFailure } from '@/features/hosts/lib/host-failure-notice'
import {
  getEditorHost,
  loadEditorHost,
  releaseWithDocument,
} from '@/features/editor/host/editor-host'
import { documentName, documentRefOf, documentUri, sameDocument } from './document-ref'

type HostRef = Extract<DocumentRef, { authority: 'host' }>

export interface OpenDocumentOptions {
  preview?: boolean
  paneId?: string
  beforeShow?: (bufferId: string) => void
  onRefused?: (reason: DocumentUnavailable) => void
}

export function findDocumentBuffer(ref: DocumentRef): EditorContent | null {
  const found = windowPaneStore
    .getState()
    .buffers.find((b) => isEditorContent(b) && !b.isVirtual && sameDocument(documentRefOf(b), ref))
  return found && isEditorContent(found) ? found : null
}

function viewKeyOf(paneId: string): string | null {
  const home = homeOf(windowPaneStore.getState(), paneId)
  return home && (home.kind === 'view' ? `view:${home.viewId}` : home.kind)
}

function show(buffer: EditorContent, { paneId, beforeShow }: OpenDocumentOptions): string {
  beforeShow?.(buffer.id)
  const { panes, paneActions, activePaneId } = windowPaneStore.getState()
  const target = paneId ?? activePaneId
  const view = viewKeyOf(target)
  const holder = Object.values(panes).find(
    (p) => p.editorTabIds.includes(buffer.id) && view !== null && viewKeyOf(p.id) === view,
  )
  if (holder) {
    paneActions.setActivePane(holder.id)
    paneActions.activateEditorTabInPane(holder.id, buffer.id)
  } else {
    paneActions.addEditorTabToPane(target, buffer)
  }
  return buffer.id
}

function targetLabel(target: DocumentTarget): { path: string; name: string } {
  const path = 'absolutePath' in target ? target.absolutePath : target.path
  const name = path.split('/').pop() || path
  return { path, name: name.replace(/^untitled:/, '') }
}

export function documentRefusal(error: unknown): { title: string; message: string } | null {
  if (!(error instanceof ApiError) || error.status !== 403) return null
  if (error.problem.code === HOST_DOCUMENT_TRANSPORT_CODE) {
    return {
      title: "Can't open this file here",
      message: 'Files outside your workspaces open only in the Crowbar desktop app.',
    }
  }
  if (error.problem.code === DAEMON_STATE_DOCUMENT_CODE) {
    return { title: "Can't open this file", message: "It holds Crowbar's own state." }
  }
  return null
}

export function openFailure(target: DocumentTarget, error: unknown): DocumentUnavailable {
  const { path, name } = targetLabel(target)
  const refusal = documentRefusal(error)
  const status = error instanceof ApiError ? error.status : 0
  if (refusal) return refusal
  if (status === 403) return { title: "Can't open file: permission denied", message: name }
  if (status === 404) return { title: 'File not found', message: path }
  if (status === 413) return { title: 'File too large to open', message: name }
  return { title: 'Failed to open file', message: name }
}

function reportOpenFailure(
  target: DocumentTarget,
  error: unknown,
  onRefused: OpenDocumentOptions['onRefused'],
): void {
  if (!onRefused && toastHostFailure(error)) return
  const hostRefusal = onRefused ? hostFailureRefusal(error) : null
  sayRefused(hostRefusal ?? openFailure(target, error), onRefused)
}

function sayRefused(reason: DocumentUnavailable, onRefused: OpenDocumentOptions['onRefused']) {
  if (onRefused) return onRefused(reason)
  toast.error(reason.title, reason.message)
}

export function watchRefusal(
  ref: DocumentRef,
  read: { ref: DocumentRef } | { error: unknown },
  refusal: unknown,
): DocumentUnavailable {
  if ('error' in read) return openFailure(ref, read.error)
  if (isWatchLimit(refusal)) {
    return { title: "Can't watch this file: too many file watches", message: documentName(ref) }
  }
  if (!sameDocument(read.ref, ref)) return { title: 'Failed to open file', message: ref.path }
  return openFailure(ref, null)
}

export function setDocumentUnavailable(ref: DocumentRef, reason: DocumentUnavailable | null): void {
  const holder = windowPaneStore
    .getState()
    .buffers.find(
      (b): b is EditorContent =>
        isEditorContent(b) && !!b.path && sameDocument(documentRefOf(b), ref),
    )
  const shown = holder?.unavailable
  if (!holder || (shown?.title === reason?.title && shown?.message === reason?.message)) return
  const unavailable = reason ?? undefined
  windowPaneStore.setState((state) => ({
    buffers: state.buffers.map((b) => (b.id === holder.id ? { ...b, unavailable } : b)),
  }))
}

export function awaitsRetry(buffer: EditorContent): boolean {
  return !!buffer.unavailable && !getEditorHost()?.documents.has(documentUri(documentRefOf(buffer)))
}

export async function retryLoad(buffer: EditorContent): Promise<boolean> {
  const { retryDocument } = await import('./restore-documents')
  const reason = await retryDocument(buffer)
  if (reason) toast.error(reason.title, reason.message)
  return !reason
}

async function showAndRetry(
  buffer: EditorContent,
  opts: OpenDocumentOptions,
): Promise<string | null> {
  const id = show(buffer, opts)
  if (awaitsRetry(buffer)) return (await retryLoad(buffer)) ? id : null
  if (buffer.authority === 'host') {
    const { refollowHostDocument } = await import('@/features/workspace/lib/external-buffer-sync')
    await refollowHostDocument(buffer)
  }
  return id
}

export type OpenDocumentModel = (ref: DocumentRef, text: string, savedText?: string) => void

export async function documentModelOpener(): Promise<OpenDocumentModel> {
  const host = await loadEditorHost()
  return (ref, text, savedText = text) => {
    const uri = documentUri(ref)
    host.documents.open(ref, host.languageOf(uri), text, savedText)
  }
}

interface Loaded {
  read: DocumentRead
  openModel: OpenDocumentModel
}

async function load(
  target: DocumentTarget,
  onRefused: OpenDocumentOptions['onRefused'],
): Promise<Loaded | null> {
  try {
    const [read, openModel] = await Promise.all([readDocument(target), documentModelOpener()])
    return { read, openModel }
  } catch (error) {
    reportOpenFailure(target, error, onRefused)
    return null
  }
}

function openLoaded({ read, openModel }: Loaded, opts: OpenDocumentOptions): string | null {
  openModel(read.ref, read.text)
  const id = windowPaneStore
    .getState()
    .bufferActions.openContent(
      { type: 'editor', doc: read.ref, name: documentName(read.ref), isPreview: opts.preview },
      { paneId: opts.paneId },
    )
  if (!id) {
    getEditorHost()?.documents.release(documentUri(read.ref))
    return null
  }
  opts.beforeShow?.(id)
  return id
}

async function openHostDocument(ref: HostRef, opts: OpenDocumentOptions): Promise<string | null> {
  const { followHostDocument } = await import('@/features/workspace/lib/external-buffer-sync')
  let unwatch: () => void
  try {
    unwatch = await followHostDocument(ref)
  } catch (refusal) {
    const read = await readDocument(ref).catch((error: unknown) => ({ error }))
    sayRefused(watchRefusal(ref, read, refusal), opts.onRefused)
    return null
  }
  const loaded = await load(ref, opts.onRefused)
  const landed = loaded && findDocumentBuffer(loaded.read.ref)
  if (!loaded || landed) {
    unwatch()
    return landed ? showAndRetry(landed, opts) : null
  }
  const id = openLoaded(loaded, opts)
  releaseWithDocument(documentUri(loaded.read.ref), unwatch)
  return id
}

export async function openDocument(
  target: DocumentTarget,
  opts: OpenDocumentOptions = {},
): Promise<string | null> {
  const open = 'absolutePath' in target ? null : findDocumentBuffer(target)
  if (open) return showAndRetry(open, opts)
  if (!('absolutePath' in target) && target.authority === 'host') {
    return openHostDocument(target, opts)
  }
  const loaded = await load(target, opts.onRefused)
  if (!loaded) return null
  const landed = findDocumentBuffer(loaded.read.ref)
  if (landed) return showAndRetry(landed, opts)
  if (loaded.read.ref.authority === 'host') return openHostDocument(loaded.read.ref, opts)
  return openLoaded(loaded, opts)
}

export async function openDocumentsAtPaths(
  paths: readonly string[],
  paneId: string,
): Promise<void> {
  for (const absolutePath of paths) {
    // react-doctor-disable-next-line async-await-in-loop -- sequential on purpose: each open appends to the pane's tabs, so concurrent opens land out of drop order.
    await openDocument({ absolutePath }, { paneId })
  }
}

export async function openDocumentFromDialog(paneId: string): Promise<void> {
  if (!isTauri()) return
  const picked = await openNativeDialog({ multiple: true, directory: false })
  if (picked) await openDocumentsAtPaths(picked, paneId)
}

function freeUntitledRef(workspaceId: string): DocumentRef {
  const taken = new Set(windowPaneStore.getState().buffers.map((b) => b.path))
  let n = 1
  while (taken.has(`untitled:Untitled-${n}`)) n += 1
  return workspaceRef(workspaceId, `untitled:Untitled-${n}`)
}

export async function openUntitledDocument(
  workspaceId: string,
  paneId: string,
  replaceTabId?: string,
): Promise<string> {
  const openModel = await documentModelOpener()
  const doc = freeUntitledRef(workspaceId)
  openModel(doc, '')
  const id = windowPaneStore
    .getState()
    .bufferActions.openContent(
      { type: 'editor', doc, name: 'Untitled', isVirtual: true },
      { paneId, replaceTabId },
    )
  if (!id) getEditorHost()?.documents.release(documentUri(doc))
  return id
}
