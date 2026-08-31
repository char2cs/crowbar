import { useEffect, useRef } from 'react'
import { useStore } from 'zustand'
import { useJumpListStore } from '@/features/editor/stores/jump-list-store'
import { useEditorStateStore } from '@/features/editor/stores/state-store'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { getActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'

interface Origin {
  bufferId: string
  filePath: string
  workspaceId: string
}

/**
 * Records file navigation into the jump list so the sidebar's back/forward
 * arrows traverse real history.
 *
 * Previously exactly one call site pushed onto the jump list — a successful
 * go-to-definition (`use-go-to-definition.ts`). Opening files and switching tabs
 * recorded nothing, so the arrows sat permanently disabled despite being drawn
 * and positioned like browser history controls.
 *
 * Entries follow the store's existing "push where I was, then move" semantic, so
 * go-to-definition jumps and ordinary file switches share one coherent history
 * instead of two competing stacks.
 *
 * Only `editor` buffers are recorded: going back to a file whose tab was closed
 * can be satisfied by reopening it from disk (see `navigateToJumpEntry`), which
 * is not true of terminals or agent chats.
 */
export function useNavigationHistory(): void {
  // Task 26: panes/buffers are window-level now (one flat store, never
  // destroyed on workspace switch) — `activeBufferId` alone no longer implies
  // a stable "current workspace" the way it did when each workspace owned its
  // own store instance.
  const activeBufferId = useStore(
    windowPaneStore,
    (s) => s.panes[s.activePaneId]?.activeEditorTabId ?? null,
  )

  // Where the user currently is. On a switch this still holds the *outgoing*
  // buffer, which is what gets pushed — the position being left, not entered.
  const originRef = useRef<Origin | null>(null)
  // The ACTIVE WORKSPACE id the origin was read under. A workspace switch
  // (`setActiveWorkspaceId`, still driven by WorkspaceView's own effect —
  // Task 26 did not change what "active workspace" means, only where panes
  // live) can activate a different workspace's buffer in what is otherwise
  // the SAME pane — indistinguishable, from `activeBufferId` alone, from an
  // ordinary tab switch. Comparing the buffer's own `workspaceId` is what
  // tells the two apart now that there is no separate store identity to diff.
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
        ? { bufferId: buffer.id, filePath: buffer.path, workspaceId: buffer.workspaceId }
        : null

    const origin = originRef.current
    const originWorkspaceId = originWorkspaceIdRef.current
    originRef.current = incoming
    originWorkspaceIdRef.current = incoming?.workspaceId ?? activeWorkspaceId

    // This activation was performed *by* back/forward. Adopt it as the new
    // origin but do not record it — otherwise every step of history navigation
    // would append a fresh entry and the stack could never be walked twice.
    if (useJumpListStore.getState().actions.consumeNavigationTarget(activeBufferId)) return

    // Changing WORKSPACE is not navigating within one. The origin belongs to the
    // workspace being left, and the jump list is a process-global singleton with
    // workspace-relative paths, so recording here puts a foreign entry into the
    // new workspace's history — Back then opens the sibling worktree's file of
    // the same name. Adopt the incoming position (done above) and record nothing.
    const workspaceChanged =
      originWorkspaceId !== null && incoming !== null && originWorkspaceId !== incoming.workspaceId
    if (workspaceChanged) return

    if (!origin || origin.bufferId === activeBufferId) return

    const { cursorPosition, scrollTop, scrollLeft } = useEditorStateStore.getState()
    useJumpListStore.getState().actions.pushEntry({
      bufferId: origin.bufferId,
      workspaceId: origin.workspaceId,
      filePath: origin.filePath,
      line: cursorPosition.line,
      column: cursorPosition.column,
      offset: cursorPosition.offset,
      scrollTop,
      scrollLeft,
    })
  }, [activeBufferId])
}
