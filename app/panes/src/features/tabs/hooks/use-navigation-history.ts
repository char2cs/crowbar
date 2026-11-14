import { useEffect, useRef } from 'react'
import { useStore } from 'zustand'
import { useJumpListStore } from '@/features/editor/stores/jump-list-store'
import { useEditorStateStore } from '@/features/editor/stores/state-store'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { getActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'
import { documentRefOf } from '@/features/editor/documents/document-ref'
import type { DocumentRef } from '@/lib/api/documents'

interface Origin {
  bufferId: string
  doc: DocumentRef
  workspaceId: string
}

/** Records editor-file switches into the jump list ("push where I was, then move"), so the
 *  back/forward arrows and go-to-definition share one history. Only editor buffers count: a closed
 *  file can be reopened from disk; a terminal or chat cannot. */
export function useNavigationHistory(): void {
  // Panes are window-level, so the active buffer alone does not identify the workspace.
  const activeBufferId = useStore(
    windowPaneStore,
    (s) => s.panes[s.activePaneId]?.activeEditorTabId ?? null,
  )

  // Where the user currently is. On a switch this still holds the *outgoing*
  // buffer, which is what gets pushed — the position being left, not entered.
  const originRef = useRef<Origin | null>(null)
  // A workspace switch can activate another workspace's buffer in the SAME pane, which looks
  // like a tab switch; comparing the buffer's `workspaceId` against this tells them apart.
  const originWorkspaceIdRef = useRef<string | null>(null)

  useEffect(() => {
    const state = windowPaneStore.getState()
    const buffer = state.buffers.find((b) => b.id === activeBufferId)
    const activeWorkspaceId = getActiveWorkspaceId()

    if (!activeBufferId) {
      originRef.current = null
      originWorkspaceIdRef.current = activeWorkspaceId
      return
    }

    const incoming: Origin | null =
      buffer?.type === 'editor' && buffer.path
        ? { bufferId: buffer.id, doc: documentRefOf(buffer), workspaceId: buffer.workspaceId }
        : null

    const origin = originRef.current
    const originWorkspaceId = originWorkspaceIdRef.current
    originRef.current = incoming
    originWorkspaceIdRef.current = incoming?.workspaceId ?? activeWorkspaceId

    // Activated by back/forward: adopt as origin but don't record, or every step would append an
    // entry and the stack could never be walked twice.
    if (useJumpListStore.getState().actions.consumeNavigationTarget(activeBufferId)) return

    // A workspace change is not navigation. The jump list is process-global with workspace-relative
    // paths, so recording would make Back open the sibling worktree's same-named file.
    const workspaceChanged =
      originWorkspaceId !== null && incoming !== null && originWorkspaceId !== incoming.workspaceId
    if (workspaceChanged) return

    if (!origin || origin.bufferId === activeBufferId) return

    const { cursorPosition } = useEditorStateStore.getState()
    useJumpListStore.getState().actions.pushEntry({
      bufferId: origin.bufferId,
      doc: origin.doc,
      line: cursorPosition.line,
      column: cursorPosition.column,
      offset: cursorPosition.offset,
      scrollTop: 0,
      scrollLeft: 0,
    })
  }, [activeBufferId])
}
