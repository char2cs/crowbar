import { readWorkspaceFile } from '@/features/file-system/controllers/platform'
import { useFileWatcherStore } from '@/features/file-system/controllers/file-watcher-store'
import { isEditorContent } from '@/features/panes/types/pane-content'
import { toast } from '@/features/window/stores/toast-store'
import type { WorkspaceStore } from '@/features/workspace/stores/workspace-store'

/**
 * Reconcile an open editor buffer with an external on-disk change (e.g.
 * `git restore`, another tool writing the file). Matches real-IDE behaviour:
 *
 * - Own-save echoes (paths marked via markPendingSave) are consumed and
 *   ignored — the buffer already holds what was written.
 * - A clean buffer is silently reloaded from disk (content + savedContent),
 *   so a discarded file can never be resurrected by a later save.
 * - A dirty buffer is never clobbered: it is flagged with hasExternalChange
 *   and a toast tells the user the file changed underneath them.
 */
export async function syncBufferWithDisk(wsStore: WorkspaceStore, path: string): Promise<void> {
  const { pendingSaves, clearPendingSave } = useFileWatcherStore.getState()
  if (pendingSaves.has(path)) {
    // Echo of our own write — consume the marker, nothing to reconcile.
    clearPendingSave(path)
    return
  }

  const buffer = wsStore
    .getState()
    .buffers.find((b) => isEditorContent(b) && !b.isVirtual && b.path === path)
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
    // in hydrate.ts (diskContent === savedContent).
    const disk = await readWorkspaceFile(wsStore.getState().workspaceId, path).catch(() => null)
    if (disk === null) return

    // Re-read at apply time: the user may have edited/closed the buffer, or a
    // concurrent echo may have already flagged it, while the read was in flight.
    const current = wsStore
      .getState()
      .buffers.find((b) => isEditorContent(b) && !b.isVirtual && b.path === path)
    if (!current || !isEditorContent(current) || !current.isDirty || current.hasExternalChange) {
      return
    }
    if (disk === current.savedContent) return // own-write echo — nothing diverged

    wsStore.setState((state) => ({
      buffers: state.buffers.map((b) =>
        isEditorContent(b) && b.path === path && b.isDirty ? { ...b, hasExternalChange: true } : b,
      ),
    }))
    toast.warning(
      `${current.name} changed on disk`,
      'Your unsaved changes are kept. Saving will overwrite the on-disk version.',
    )
    return
  }

  // Read from the buffer's own workspace, not the active one: this runs from
  // async flows (FS events, restore reconciliation) that can complete after
  // the user switched workspaces, and the same relative path in a sibling
  // worktree holds different content.
  const content = await readWorkspaceFile(wsStore.getState().workspaceId, path).catch(() => null)
  if (content === null) return

  // Re-check at apply time: the user may have started editing (or closed the
  // buffer) while the read was in flight — never clobber fresh edits.
  wsStore.setState((state) => ({
    buffers: state.buffers.map((b) =>
      isEditorContent(b) && !b.isVirtual && b.path === path && !b.isDirty
        ? {
            ...b,
            content,
            savedContent: content,
            isDirty: false,
            hasExternalChange: false,
            fileMissing: false,
          }
        : b,
    ),
  }))
}
