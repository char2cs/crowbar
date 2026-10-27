import { isNotFoundError } from '@/lib/api'
import {
  documentWatchEndpoint,
  readDocument,
  readDocumentFromDisk,
  type DocumentRef,
} from '@/lib/api/documents'
import { useConnectionStore } from '@/lib/ws/connection-store'
import { wsManager } from '@/lib/ws/manager'
import { isReconnectSentinel } from '@/lib/ws/types'
import {
  documentName,
  documentRefOf,
  documentUri,
  sameDocument,
} from '@/features/editor/documents/document-ref'
import {
  applyExternalText,
  documentSaveMark,
  documentText,
  isDocumentDirty,
  savedDocumentText,
} from '@/features/editor/documents/document-text'
import {
  awaitsRetry,
  documentRefusal,
  openFailure,
  setDocumentUnavailable,
} from '@/features/editor/documents/open-document'
import { getEditorHost, releaseWithDocument } from '@/features/editor/host/editor-host'
import { useFileWatcherStore } from '@/features/file-system/controllers/file-watcher-store'
import {
  isEditorContent,
  isPersistableContent,
  type EditorContent,
  type PaneContent,
} from '@/features/panes/types/pane-content'
import { toast } from '@/features/window/stores/toast-store'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'

type HostRef = Extract<DocumentRef, { authority: 'host' }>

const holding = (ref: DocumentRef) => (b: PaneContent) =>
  isEditorContent(b) && !b.isVirtual && sameDocument(documentRefOf(b), ref)

/** A dirty buffer whose file changed on disk keeps its edits; it is flagged and the user told once. */
export function flagExternalChange(ref: DocumentRef): void {
  const holder = windowPaneStore.getState().buffers.find(holding(ref))
  if (!holder || !isEditorContent(holder) || !holder.isDirty || holder.hasExternalChange) return
  windowPaneStore.setState((state) => ({
    buffers: state.buffers.map((b) =>
      b.id === holder.id && isEditorContent(b) ? { ...b, hasExternalChange: true } : b,
    ),
  }))
  toast.warning(
    `${holder.name} changed on disk`,
    'Your unsaved changes are kept. Saving will overwrite the on-disk version.',
  )
}

/**
 * Disk text read for an open document, begun at save mark `since`: a clean document takes
 * it as saved; a dirty one keeps its edits and is flagged only when disk holds neither its
 * saved text nor its own. A read older than the document's latest save or open never lands.
 */
export function takeDiskText(ref: DocumentRef, disk: string, since: number): void {
  const text = documentText(ref)
  if (text === null || documentSaveMark(ref) !== since) return
  if (!isDocumentDirty(ref)) return applyExternalText(ref, disk, { saved: true })
  if (disk !== savedDocumentText(ref) && disk !== text) flagExternalChange(ref)
}

/**
 * Reconcile an open editor buffer with an external on-disk change (`git restore`, another
 * tool writing the file): own-save echoes (marked via markPendingSave) are consumed, and
 * whatever disk now holds goes through `takeDiskText`.
 */
export async function syncBufferWithDisk(ref: DocumentRef): Promise<void> {
  const { pendingSaves, clearPendingSave } = useFileWatcherStore.getState()
  if (pendingSaves.has(ref.path)) {
    clearPendingSave(ref.path)
    return
  }
  // Buffers are one window-level list: a sibling worktree can hold another
  // file at the same relative path, so every lookup matches the whole document.
  const buffer = windowPaneStore.getState().buffers.find(holding(ref))
  if (!buffer || !isEditorContent(buffer)) return
  if (buffer.isDirty && buffer.hasExternalChange) return // already flagged, don't re-toast
  const since = documentSaveMark(ref)
  const disk = await readDocumentFromDisk(ref)
  if (disk !== null) takeDiskText(ref, disk, since)
}

const changesText = (frame: unknown) => {
  const type = (frame as { type?: unknown } | null)?.type
  return type === 'modified' || type === 'created'
}

/**
 * Follows a host document's file through its own watch socket. Resolves with the way to stop
 * once the daemon has armed the watch, so a read made after it misses no change; rejects, holding
 * nothing, when the socket closes before it opens.
 */
export async function followHostDocument(ref: HostRef): Promise<() => void> {
  const endpoint = documentWatchEndpoint(ref.path)
  let following = true
  // A reopened socket is a re-armed watch: what changed while it was down is read once.
  const unsubscribe = wsManager.subscribe(endpoint, (frame) => {
    if (isReconnectSentinel(frame) || changesText(frame)) void syncBufferWithDisk(ref)
  })
  const stopNoticing = wsManager.onDrop(endpoint, () => {
    void noticeRefusal(endpoint, ref, () => following)
  })
  const stop = () => {
    if (!following) return
    following = false
    stopNoticing()
    unsubscribe()
  }
  try {
    await wsManager.opened(endpoint)
  } catch (error) {
    stop()
    throw error
  }
  return stop
}

/**
 * The manager retries a dropped socket forever. Two failed reconnects in a row while the
 * daemon answers other streams mean it refuses this watch: the user is told once per drop.
 */
async function noticeRefusal(
  endpoint: string,
  ref: HostRef,
  following: () => boolean,
  refusals = 0,
): Promise<void> {
  const reopened = await wsManager.opened(endpoint).then(
    () => true,
    () => false,
  )
  if (reopened || !following()) return
  if (useConnectionStore.getState().status !== 'connected') {
    return noticeRefusal(endpoint, ref, following)
  }
  if (refusals === 0) return noticeRefusal(endpoint, ref, following, 1)
  toast.warning(
    `Not following changes to ${documentName(ref)}`,
    "Crowbar can't watch this file right now and keeps trying.",
  )
}

/** A host document open without its watch follows again, then reads what it missed. */
async function followAgain(ref: HostRef): Promise<void> {
  let unwatch: () => void
  try {
    unwatch = await followHostDocument(ref)
  } catch {
    return // Still refused: the tab already said so.
  }
  releaseWithDocument(documentUri(ref), unwatch)
  await syncBufferWithDisk(ref)
}

/** A host document open while no watch holds it: it opened when its follow was refused. */
function heldUnwatched(buffer: EditorContent): boolean {
  const ref = documentRefOf(buffer)
  return (
    ref.authority === 'host' &&
    !!getEditorHost()?.documents.has(documentUri(ref)) &&
    !wsManager.subscribed(documentWatchEndpoint(ref.path))
  )
}

/** Reopening a host document left unwatched follows it again; any other is left alone. */
export async function refollowHostDocument(buffer: EditorContent): Promise<void> {
  const ref = documentRefOf(buffer)
  if (ref.authority === 'host' && heldUnwatched(buffer)) await followAgain(ref)
}

/**
 * Reconcile a workspace's documents with disk after it sat hidden (its watcher is
 * active-only). Only disk diverging from the saved text counts, or every dirty buffer
 * would warn on each activation; a missing file that came back reopens. A host document
 * is its watch's alone, unless it holds none: then it follows again.
 */
export async function reconcileWorkspaceDocuments(workspaceId: string): Promise<void> {
  const refusals = new Map<string, string>()
  // An untitled tab has no file to sync, but one that failed to load retries too.
  const buffers = windowPaneStore
    .getState()
    .buffers.filter(
      (b): b is EditorContent =>
        isEditorContent(b) &&
        (b.authority === 'host'
          ? awaitsRetry(b) || heldUnwatched(b)
          : b.workspaceId === workspaceId && (isPersistableContent(b) || awaitsRetry(b))),
    )
  await Promise.allSettled(buffers.map((buffer) => reconcileDocument(buffer, refusals)))
  for (const [title, message] of refusals) toast.error(title, message)
}

async function reconcileDocument(buffer: EditorContent, refusals: Map<string, string>) {
  const ref = documentRefOf(buffer)
  // A tab that failed to load reads again, silently: it already says why.
  if (awaitsRetry(buffer)) {
    // Lazy: the restore builds on this module.
    const { retryDocument } = await import('@/features/editor/documents/restore-documents')
    await retryDocument(buffer)
    return
  }
  if (ref.authority === 'host') return followAgain(ref)
  // Not open yet: the restore has not reached it.
  if (savedDocumentText(ref) === null) return
  let disk: string
  try {
    disk = (await readDocument(ref)).text
  } catch (err) {
    const refusal = documentRefusal(err)
    if (refusal) refusals.set(refusal.title, refusal.message)
    else if (isNotFoundError(err) && !buffer.isDirty)
      setDocumentUnavailable(ref, openFailure(ref, err))
    return
  }
  setDocumentUnavailable(ref, null)
  if (disk !== savedDocumentText(ref)) await syncBufferWithDisk(ref)
}
