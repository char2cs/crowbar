import { isNotFoundError } from '@/lib/api'
import { readDocument, readDocumentFromDisk, type DocumentRef } from '@/lib/api/documents'
import { documentRefOf, sameDocument } from '@/features/editor/documents/document-ref'
import { applyExternalText, savedDocumentText } from '@/features/editor/documents/document-text'
import { documentRefusal } from '@/features/editor/documents/open-document'
import { useFileWatcherStore } from '@/features/file-system/controllers/file-watcher-store'
import {
  isEditorContent,
  isPersistableContent,
  type EditorContent,
  type PaneContent,
} from '@/features/panes/types/pane-content'
import { toast } from '@/features/window/stores/toast-store'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'

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
 * Reconcile an open editor buffer with an external on-disk change (e.g.
 * `git restore`, another tool writing the file). Matches real-IDE behaviour:
 *
 * - Own-save echoes (paths marked via markPendingSave) are consumed and
 *   ignored — the buffer already holds what was written.
 * - A clean buffer is silently reloaded from disk as saved text, so a
 *   discarded file can never be resurrected by a later save.
 * - A dirty buffer is never clobbered: it is flagged with hasExternalChange
 *   and a toast tells the user the file changed underneath them.
 */
export async function syncBufferWithDisk(ref: DocumentRef): Promise<void> {
  const { path } = ref
  const holds = holding(ref)
  const { pendingSaves, clearPendingSave } = useFileWatcherStore.getState()
  if (pendingSaves.has(path)) {
    // Echo of our own write — consume the marker, nothing to reconcile.
    clearPendingSave(path)
    return
  }

  // Buffers are one window-level list: a sibling worktree can hold another
  // file at the same relative path, so every lookup matches the whole document.
  const buffer = windowPaneStore.getState().buffers.find(holds)
  if (!buffer || !isEditorContent(buffer)) return

  if (buffer.isDirty) {
    if (buffer.hasExternalChange) return // already flagged, don't re-toast

    // The pendingSaves marker above is a single-shot flag: it cancels only the
    // FIRST FS event after a write. A single editor save can surface as several
    // "modified" events (macOS FSEvents coalescing/splitting), and the backend
    // rebroadcasts each one with no echo suppression — so a straggler slips past
    // the consumed marker. Before warning, confirm the on-disk bytes actually
    // diverged from the baseline we last wrote; if they match, this is our own
    // write echoing back, not an external edit. Mirrors the restore-path guard
    // in reconcileBuffer below (disk === saved text).
    const disk = await readDocumentFromDisk(ref)
    if (disk === null || disk === savedDocumentText(ref)) return // own-write echo — nothing diverged

    // Re-checked at apply time: the buffer may have been saved, closed or
    // already flagged while the read was in flight.
    flagExternalChange(ref)
    return
  }

  const content = await readDocumentFromDisk(ref)
  if (content === null) return

  // Re-check at apply time: the user may have started editing (or closed the
  // buffer) while the read was in flight — never clobber fresh edits.
  const current = windowPaneStore.getState().buffers.find(holds)
  if (current && isEditorContent(current) && !current.isDirty) {
    applyExternalText(ref, content, { saved: true })
  }
}

/**
 * Restored buffers may be stale: the file can change on disk while the app is
 * closed. Each is reconciled with the policy of a live external change, but
 * only when disk diverges from the saved text — otherwise every dirty buffer
 * would get a false "changed on disk" toast on every reload.
 *
 * A file gone from disk marks a clean buffer missing (a dirty one keeps the
 * only copy of its edits). A file the daemon refuses this client keeps its tab
 * and its reason is shown once; other failures stay silent and non-terminal.
 */
export async function reconcileBuffersWithDisk(buffers: PaneContent[]): Promise<void> {
  const refusals = new Map<string, string>()
  const reconciles: Promise<void>[] = []
  for (const buffer of buffers) {
    if (isPersistableContent(buffer)) reconciles.push(reconcileBuffer(buffer, refusals))
  }
  await Promise.allSettled(reconciles)
  for (const [title, message] of refusals) toast.error(title, message)
}

async function reconcileBuffer(
  buffer: EditorContent,
  refusals: Map<string, string>,
): Promise<void> {
  const ref = documentRefOf(buffer)
  let disk: string
  try {
    disk = (await readDocument(ref)).text
  } catch (err) {
    const refusal = documentRefusal(err)
    if (refusal) refusals.set(refusal.title, refusal.message)
    else if (isNotFoundError(err) && !buffer.isDirty) setBufferFileMissing(ref, true)
    return
  }
  if (buffer.fileMissing) setBufferFileMissing(ref, false)
  if (disk !== savedDocumentText(ref)) await syncBufferWithDisk(ref)
}

/** Host documents restored with the window: no workspace activation reconciles them. */
export function reconcileHostBuffersWithDisk(): Promise<void> {
  return reconcileBuffersWithDisk(
    windowPaneStore.getState().buffers.filter((b) => isEditorContent(b) && b.authority === 'host'),
  )
}

function setBufferFileMissing(ref: DocumentRef, fileMissing: boolean): void {
  windowPaneStore.setState((state) => ({
    buffers: state.buffers.map((b) => (holding(ref)(b) ? { ...b, fileMissing } : b)),
  }))
}
