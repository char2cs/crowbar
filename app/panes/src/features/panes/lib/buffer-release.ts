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
import { useMarkdownViewStore } from '@/features/editor/markdown/plate/markdown-view-store'
import { report } from '@/lib/errors/report'
import { browserApi } from '@/lib/browser-bridge'
import { unregisterWebview } from '@/features/browser/lib/webview-registry'

export interface BufferOwnershipState {
  panes: Record<string, Pick<PaneGroup, 'editorTabIds'>>
  buffers: PaneContent[]
  closedBuffersHistory: ClosedBuffer[]
}

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

function releaseDocumentOf(buf: PaneContent): void {
  if (!isEditorContent(buf) || !buf.path) return
  getEditorHost()?.documents.release(documentUri(documentRefOf(buf)))
}

export function disposeBuffers(released: readonly PaneContent[]): void {
  for (const buf of released) {
    if (buf.type === 'terminal') {
      const { sessionId, workspaceId } = buf as TerminalContent
      void stopTerminal(sessionId, workspaceId).catch((err: unknown) =>
        report(err, { action: 'Stop the closed terminal' }),
      )
    }
    releaseDocumentOf(buf)
    const webview = buf.type === 'browser' ? unregisterWebview(buf.id) : undefined
    if (webview) {
      void webview
        .then(() => browserApi.close(buf.id))
        .catch((err: unknown) => report(err, { action: 'Close the browser tab' }))
    }
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
  clearReconnect(workspaceId, sessionId)
  await killTerminalSession(sessionId)
}
