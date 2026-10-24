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
import {
  isEditorContent,
  type EditorContent,
  type PaneContent,
} from '@/features/panes/types/pane-content'
import { toast } from '@/features/window/stores/toast-store'
import { documentName, documentRefOf, sameDocument } from './document-ref'

export interface OpenDocumentOptions {
  preview?: boolean
  /** The pane the tab lands in; omitted, the pane already showing it or the focused one. */
  paneId?: string
  /** Called with the buffer about to be shown, before anything observes it shown. */
  beforeShow?: (bufferId: string) => void
}

/** The editor buffer in `buffers` holding `ref`, if any. */
export function documentBufferIn(
  buffers: readonly PaneContent[],
  ref: DocumentRef,
): EditorContent | null {
  const found = buffers.find(
    (b) => isEditorContent(b) && !b.isVirtual && sameDocument(documentRefOf(b), ref),
  )
  return found && isEditorContent(found) ? found : null
}

/** The open editor buffer holding `ref`, if any. */
export function findDocumentBuffer(ref: DocumentRef): EditorContent | null {
  return documentBufferIn(windowPaneStore.getState().buffers, ref)
}

function show(buffer: EditorContent, { paneId, beforeShow }: OpenDocumentOptions): string {
  beforeShow?.(buffer.id)
  const { panes, paneActions } = windowPaneStore.getState()
  const holder = Object.values(panes).find((p) => p.editorTabIds.includes(buffer.id))
  if (paneId && paneId !== holder?.id) {
    paneActions.addEditorTabToPane(paneId, buffer)
  } else if (holder) {
    paneActions.setActivePane(holder.id)
    paneActions.activateEditorTabInPane(holder.id, buffer.id)
  } else {
    paneActions.addEditorTabToPane(windowPaneStore.getState().activePaneId, buffer)
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
  let read
  try {
    read = await readDocument(target)
  } catch (error) {
    reportOpenFailure(target, error)
    return null
  }
  // A concurrent open of the same document, or another spelling of it, may have landed.
  const landed = findDocumentBuffer(read.ref)
  if (landed) return show(landed, opts)
  const id = windowPaneStore.getState().bufferActions.openContent(
    {
      type: 'editor',
      doc: read.ref,
      name: documentName(read.ref),
      content: read.text,
      isPreview: opts.preview,
    },
    { paneId: opts.paneId },
  )
  if (!id) return null
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

/** A new, unsaved document in `workspaceId`, opened into `paneId`. */
export function openUntitledDocument(workspaceId: string, paneId: string): Promise<string> {
  return Promise.resolve(
    windowPaneStore.getState().bufferActions.openContent(
      {
        type: 'editor',
        doc: workspaceRef(workspaceId, 'untitled:Untitled'),
        name: 'Untitled',
        content: '',
        isVirtual: true,
      },
      { paneId },
    ),
  )
}
