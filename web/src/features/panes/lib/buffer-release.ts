import { current, isDraft } from 'immer'
import type { PaneGroup } from '@/features/panes/types/pane'
import {
  isEditorContent,
  shouldStartLsp,
  type ClosedBuffer,
  type PaneContent,
  type TerminalContent,
} from '@/features/panes/types/pane-content'
import { EDITOR_CONSTANTS } from '@/features/editor/config/constants'
import { useHistoryStore } from '@/features/editor/stores/history-store'
import { cleanupBufferHistoryTracking } from '@/features/editor/stores/buffer-history-tracking'
// Leaf module (zustand only, no Plate) — a static import keeps the rich
// editor's chunk out of the base bundle while still releasing the buffer's
// rich/source preference synchronously.
import { useMarkdownViewStore } from '@/features/editor/markdown/plate/markdown-view-store'
import { bestEffort } from '@/lib/best-effort'

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
    if (!shouldStartLsp(buf)) continue
    state.closedBuffersHistory.unshift({
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

/**
 * Free everything a released buffer held outside the store: a terminal's PTY
 * (terminals never enter the reopen history, so the shell dies here or it
 * leaks), git-blame data, the markdown view preference and undo history.
 */
export function disposeBuffers(released: readonly PaneContent[]): void {
  for (const buf of released) {
    if (buf.type === 'terminal') {
      const { sessionId, workspaceId } = buf as TerminalContent
      bestEffort(
        import('@/features/terminal/lib/kill-terminal-session').then(
          async ({ killTerminalSession }) => {
            await killTerminalSession(sessionId).catch(() => {})
            // A stale connectionId must not be picked up if the same tab
            // sessionId is reused in a later session.
            const { clearReconnect } =
              await import('@/features/terminal/lib/terminal-reconnect-map')
            clearReconnect(workspaceId, sessionId)
          },
        ),
        'kill terminal session',
      )
    }
    if (isEditorContent(buf) && buf.path) {
      const filePath = buf.path
      bestEffort(
        import('@/features/git/stores/git-blame-store').then(({ useGitBlameStore }) => {
          useGitBlameStore.getState().clearBlameForFile(filePath)
        }),
        'clear blame for closed buffer',
      )
    }
    useMarkdownViewStore.getState().clearView(buf.id)
    cleanupBufferHistoryTracking(buf.id)
    useHistoryStore.getState().actions.clearHistory(buf.id)
  }
}
