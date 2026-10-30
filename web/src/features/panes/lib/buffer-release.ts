import { current, isDraft } from 'immer'
import type { PaneGroup } from '@/features/panes/types/pane'
import {
  isEditorContent,
  isPersistableContent,
  type ClosedBuffer,
  type PaneContent,
  type TerminalContent,
} from '@/features/panes/types/pane-content'
import { EDITOR_CONSTANTS } from '@/features/editor/config/constants'
import { documentRefOf, documentUri } from '@/features/editor/documents/document-ref'
import { clearDraft } from '@/features/editor/documents/draft-store'
import { getEditorHost } from '@/features/editor/host/editor-host'
// Leaf module (zustand only, no Plate) — a static import keeps the rich
// editor's chunk out of the base bundle while still releasing the buffer's
// rich/source preference synchronously.
import { useMarkdownViewStore } from '@/features/editor/markdown/plate/markdown-view-store'
import { report } from '@/lib/errors/report'

/** The slice of window-pane state buffer ownership reads and writes. */
export interface BufferOwnershipState {
  panes: Record<string, Pick<PaneGroup, 'editorTabIds'>>
  buffers: PaneContent[]
  closedBuffersHistory: ClosedBuffer[]
}

/**
 * Invariant C2: a buffer exists only while some pane lists it. Drops every
 * buffer no pane references — recording file buffers in the reopen history —
 * and returns them for `disposeBuffers`. Called inside the same `set` as the
 * pane write that unreferenced them, so no observer ever sees an orphan.
 */
export function releaseUnreferencedBuffers(state: BufferOwnershipState): PaneContent[] {
  if (!Array.isArray(state.buffers) || state.buffers.length === 0) return []
  const referenced = new Set<string>()
  for (const pane of Object.values(state.panes)) {
    for (const id of pane.editorTabIds) referenced.add(id)
  }
  const released: PaneContent[] = []
  const kept: PaneContent[] = []
  for (const buf of state.buffers) (referenced.has(buf.id) ? kept : released).push(buf)
  if (released.length === 0) return released
  state.buffers = kept
  // Detached copies: drafts are revoked once the recipe returns.
  const detached = released.map((buf) => (isDraft(buf) ? current(buf) : buf))
  if (!Array.isArray(state.closedBuffersHistory)) return detached
  for (const buf of released) {
    if (!isPersistableContent(buf)) continue
    state.closedBuffersHistory.unshift({
      authority: buf.authority,
      path: buf.path ?? '',
      name: buf.name,
      isPinned: buf.isPinned ?? false,
      workspaceId: buf.workspaceId,
    })
  }
  if (state.closedBuffersHistory.length > EDITOR_CONSTANTS.MAX_CLOSED_BUFFERS_HISTORY) {
    state.closedBuffersHistory.length = EDITOR_CONSTANTS.MAX_CLOSED_BUFFERS_HISTORY
  }
  return detached
}

/** An editor buffer's document closes with it: its model and undo history go. */
export function releaseDocumentOf(buf: PaneContent): void {
  if (!isEditorContent(buf) || !buf.path) return
  getEditorHost()?.documents.release(documentUri(documentRefOf(buf)))
}

/**
 * Free everything a released buffer held outside the store: a terminal's PTY
 * (terminals never enter the reopen history, so the shell dies here or it
 * leaks), an editor's document (its model, undo history and draft), git-blame
 * data and the markdown view preference.
 */
export function disposeBuffers(released: readonly PaneContent[]): void {
  for (const buf of released) {
    if (buf.type === 'terminal') {
      const { sessionId, workspaceId } = buf as TerminalContent
      void stopTerminal(sessionId, workspaceId).catch((err: unknown) =>
        report(err, { action: 'Stop the closed terminal' }),
      )
    }
    releaseDocumentOf(buf)
    if (isEditorContent(buf) && buf.path) {
      clearDraft(documentUri(documentRefOf(buf)))
      const { path: filePath, workspaceId } = buf
      void import('@/features/git/stores/git-blame-store')
        .then(({ clearBlame }) => clearBlame(workspaceId, filePath))
        .catch((err: unknown) => report(err, { action: 'Clear blame for the closed file' }))
    }
    useMarkdownViewStore.getState().clearView(buf.id)
  }
}

async function stopTerminal(sessionId: string, workspaceId: string): Promise<void> {
  const [{ killTerminalSession }, { clearReconnect }] = await Promise.all([
    import('@/features/terminal/lib/kill-terminal-session'),
    import('@/features/terminal/lib/terminal-reconnect-map'),
  ])
  // A later tab reusing this sessionId must not pick up the dead connection.
  clearReconnect(workspaceId, sessionId)
  await killTerminalSession(sessionId)
}
