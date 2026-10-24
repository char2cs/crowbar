import { ApiError } from '@/lib/api'
import {
  DAEMON_STATE_DOCUMENT_CODE,
  HOST_DOCUMENT_TRANSPORT_CODE,
  readDocument,
  workspaceRef,
  type DocumentRef,
  type DocumentTarget,
} from '@/lib/api/documents'
import { isTauri } from '@/lib/crowbar-bridge'
import { openNativeDialog } from '@/lib/native-dialog'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { homeOf } from '@/features/panes/lib/view-state'
import { isEditorContent, type EditorContent } from '@/features/panes/types/pane-content'
import { toast } from '@/features/window/stores/toast-store'
import { getEditorHost, loadEditorHost } from '@/features/editor/host/editor-host'
import { documentName, documentRefOf, documentUri, sameDocument } from './document-ref'

export interface OpenDocumentOptions {
  preview?: boolean
  /** The pane a tab lands in, unless a pane of its view holds it; omitted, the focused one. */
  paneId?: string
  /** Called with the buffer about to be shown, before anything observes it shown. */
  beforeShow?: (bufferId: string) => void
}

/** The open editor buffer holding `ref`, if any. */
export function findDocumentBuffer(ref: DocumentRef): EditorContent | null {
  const found = windowPaneStore
    .getState()
    .buffers.find((b) => isEditorContent(b) && !b.isVirtual && sameDocument(documentRefOf(b), ref))
  return found && isEditorContent(found) ? found : null
}

/** The view, stage or bottom tray `paneId` lives in, as one comparable key. */
function viewKeyOf(paneId: string): string | null {
  const home = homeOf(windowPaneStore.getState(), paneId)
  return home && (home.kind === 'view' ? `view:${home.viewId}` : home.kind)
}

// A holder in another view is off screen: the target pane gets its own tab.
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
  return { path, name: path.split('/').pop() || path }
}

/** Why the daemon refuses this client a document; null for any other failure. */
export function documentRefusal(error: unknown): { title: string; message: string } | null {
  if (!(error instanceof ApiError) || error.status !== 403) return null
  if (error.code === HOST_DOCUMENT_TRANSPORT_CODE) {
    return {
      title: "Can't open this file here",
      message: 'Files outside your workspaces open only in the Crowbar desktop app.',
    }
  }
  if (error.code === DAEMON_STATE_DOCUMENT_CODE) {
    return { title: "Can't open this file", message: "It holds Crowbar's own state." }
  }
  return null
}

function reportOpenFailure(target: DocumentTarget, error: unknown): void {
  const { path, name } = targetLabel(target)
  const refusal = documentRefusal(error)
  const status = error instanceof ApiError ? error.status : 0
  if (refusal) toast.error(refusal.title, refusal.message)
  else if (status === 404) toast.error('File not found', path)
  else if (status === 413) toast.error('File too large to open', name)
  else toast.error('Failed to open file', name)
}

/** Opens a document's model holding `text`; it is dirty while disk holds other `savedText`. */
export type OpenDocumentModel = (ref: DocumentRef, text: string, savedText?: string) => void

/** Resolves once Monaco has loaded, with the way to open document models synchronously. */
export async function documentModelOpener(): Promise<OpenDocumentModel> {
  const [host, { langForUri }] = await Promise.all([
    loadEditorHost(),
    import('@/features/editor/lib/monaco-adapters'),
  ])
  return (ref, text, savedText = text) => {
    host.documents.open(ref, langForUri(documentUri(ref)), text, savedText)
  }
}

/**
 * Open a document in an editor tab and answer its buffer id: an open buffer is
 * shown, anything else is read first. A failed read is reported and answers null.
 */
export async function openDocument(
  target: DocumentTarget,
  opts: OpenDocumentOptions = {},
): Promise<string | null> {
  const open = 'absolutePath' in target ? null : findDocumentBuffer(target)
  if (open) return show(open, opts)
  let read, openModel
  try {
    ;[read, openModel] = await Promise.all([readDocument(target), documentModelOpener()])
  } catch (error) {
    reportOpenFailure(target, error)
    return null
  }
  // A concurrent open of the same document, or another spelling of it, may have landed.
  const landed = findDocumentBuffer(read.ref)
  if (landed) return show(landed, opts)
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

/** Open each absolute path into `paneId`, one after another, in the order given. */
export async function openDocumentsAtPaths(
  paths: readonly string[],
  paneId: string,
): Promise<void> {
  for (const absolutePath of paths) {
    // react-doctor-disable-next-line async-await-in-loop -- sequential on purpose: each open appends to the pane's tabs, so concurrent opens land out of drop order.
    await openDocument({ absolutePath }, { paneId })
  }
}

/** The desktop Open File… dialog: every picked file opens into `paneId`. */
export async function openDocumentFromDialog(paneId: string): Promise<void> {
  if (!isTauri()) return
  const picked = await openNativeDialog({ multiple: true, directory: false })
  if (picked) await openDocumentsAtPaths(picked, paneId)
}

/** An untitled path no open buffer holds: each untitled tab is its own document. */
function freeUntitledRef(workspaceId: string): DocumentRef {
  const taken = new Set(windowPaneStore.getState().buffers.map((b) => b.path))
  let n = 1
  while (taken.has(`untitled:Untitled-${n}`)) n += 1
  return workspaceRef(workspaceId, `untitled:Untitled-${n}`)
}

/** A new, unsaved document in `workspaceId`, opened into `paneId`. */
export async function openUntitledDocument(workspaceId: string, paneId: string): Promise<string> {
  const openModel = await documentModelOpener()
  const doc = freeUntitledRef(workspaceId)
  openModel(doc, '')
  const id = windowPaneStore
    .getState()
    .bufferActions.openContent(
      { type: 'editor', doc, name: 'Untitled', isVirtual: true },
      { paneId },
    )
  if (!id) getEditorHost()?.documents.release(documentUri(doc))
  return id
}
