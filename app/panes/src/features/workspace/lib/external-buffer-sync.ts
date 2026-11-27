import { isNotFoundError } from '@/lib/errors/api-error'
import { holding as streamHolds, holdScope, onSignal, watchScope } from '@/core/stream/stream'
import {
  readDocument,
  readDocumentFromDisk,
  WatchRefusal,
  type DocumentRef,
} from '@/lib/api/documents'
import type { Problem } from '@/lib/errors/problem'
import { LOCAL_HOST } from '@/features/hosts/types'
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

export function takeDiskText(ref: DocumentRef, disk: string, since: number): void {
  const text = documentText(ref)
  if (text === null || documentSaveMark(ref) !== since) return
  if (!isDocumentDirty(ref)) return applyExternalText(ref, disk, { saved: true })
  if (disk !== savedDocumentText(ref) && disk !== text) flagExternalChange(ref)
}

export async function syncBufferWithDisk(ref: DocumentRef): Promise<void> {
  const { pendingSaves, clearPendingSave } = useFileWatcherStore.getState()
  if (pendingSaves.has(ref.path)) {
    clearPendingSave(ref.path)
    return
  }
  const buffer = windowPaneStore.getState().buffers.find(holding(ref))
  if (!buffer || !isEditorContent(buffer)) return
  if (buffer.isDirty && buffer.hasExternalChange) return
  const since = documentSaveMark(ref)
  const disk = await readDocumentFromDisk(ref)
  if (disk !== null) takeDiskText(ref, disk, since)
}

const changesText = (frame: unknown) => {
  const type = (frame as { type?: unknown } | null)?.type
  return type === 'modified' || type === 'created'
}

const docScope = (ref: HostRef) => `doc:${ref.path}`

export function followHostDocument(ref: HostRef): Promise<() => void> {
  const scope = docScope(ref)
  return new Promise((resolve, reject) => {
    let armed = false
    const stopSignals = onSignal(LOCAL_HOST.id, scope, 'file_change', (frame) => {
      if (changesText(frame)) void syncBufferWithDisk(ref)
    })
    const stopWatch = watchScope(LOCAL_HOST.id, scope, {
      synced: () => {
        if (armed) return void syncBufferWithDisk(ref)
        armed = true
        resolve(stop)
      },
      refused: (problem) => {
        stop()
        if (armed)
          return toast.warning(`Not following changes to ${documentName(ref)}`, refusedWhy(problem))
        reject(new WatchRefusal(problem ?? { code: 'internal', message: `${scope} was refused` }))
      },
    })
    const release = holdScope(LOCAL_HOST.id, scope)
    let stopped = false
    function stop() {
      if (stopped) return
      stopped = true
      stopWatch()
      stopSignals()
      release()
    }
  })
}

function refusedWhy(problem: Problem | undefined): string {
  return problem?.code === 'watch_limit'
    ? 'The OS has no file watches left.'
    : "Crowbar can't watch this file right now."
}

async function followAgain(ref: HostRef): Promise<void> {
  let unwatch: () => void
  try {
    unwatch = await followHostDocument(ref)
  } catch {
    return
  }
  releaseWithDocument(documentUri(ref), unwatch)
  await syncBufferWithDisk(ref)
}

function heldUnwatched(buffer: EditorContent): boolean {
  const ref = documentRefOf(buffer)
  return (
    ref.authority === 'host' &&
    !!getEditorHost()?.documents.has(documentUri(ref)) &&
    !streamHolds(LOCAL_HOST.id, docScope(ref))
  )
}

export async function refollowHostDocument(buffer: EditorContent): Promise<void> {
  const ref = documentRefOf(buffer)
  if (ref.authority === 'host' && heldUnwatched(buffer)) await followAgain(ref)
}

export async function reconcileWorkspaceDocuments(workspaceId: string): Promise<void> {
  const refusals = new Map<string, string>()
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
  if (awaitsRetry(buffer)) {
    const { retryDocument } = await import('@/features/editor/documents/restore-documents')
    await retryDocument(buffer)
    return
  }
  if (ref.authority === 'host') return followAgain(ref)
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
