import { useEffect, useRef } from 'react'
import { useJumpListStore } from '@/features/editor/stores/jump-list-store'
import { useEditorStateStore } from '@/features/editor/stores/state-store'
import { useActiveWorkspaceState } from '@/features/workspace/stores/hooks/use-active-workspace-state'
import type { WorkspaceStore } from '@/features/workspace/stores/workspace-store'
import { getActiveWorkspaceStoreRef } from '@/features/workspace/stores/workspace-store-ref'

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
  const activeBufferId = useActiveWorkspaceState(
    (s) => s.panes[s.activePaneId]?.activeBufferId ?? null,
    null,
  )

  // Where the user currently is. On a switch this still holds the *outgoing*
  // buffer, which is what gets pushed — the position being left, not entered.
  const originRef = useRef<Origin | null>(null)
  // The workspace store the origin was read from. A workspace switch tears down
  // the outgoing store and activates the incoming one in one batch, so this
  // effect re-runs ONCE with the new workspace's activeBufferId while the origin
  // still describes the old workspace's buffer — indistinguishable, from
  // `activeBufferId` alone, from an ordinary tab switch. Comparing store
  // IDENTITY is what tells the two apart.
  const storeRef = useRef<WorkspaceStore | null>(null)

  useEffect(() => {
    const store = getActiveWorkspaceStoreRef()
    const workspaceChanged = storeRef.current !== store
    storeRef.current = store

    if (!activeBufferId) {
      originRef.current = null
      return
    }

    const state = store?.getState()
    const buffer = state?.buffers.find((b) => b.id === activeBufferId)
    const incoming: Origin | null =
      buffer?.type === 'editor' && buffer.path && state
        ? { bufferId: buffer.id, filePath: buffer.path, workspaceId: state.workspaceId }
        : null

    const origin = originRef.current
    originRef.current = incoming

    // This activation was performed *by* back/forward. Adopt it as the new
    // origin but do not record it — otherwise every step of history navigation
    // would append a fresh entry and the stack could never be walked twice.
    if (useJumpListStore.getState().actions.consumeNavigationTarget(activeBufferId)) return

    // Changing WORKSPACE is not navigating within one. The origin belongs to the
    // workspace being left, and the jump list is a process-global singleton with
    // workspace-relative paths, so recording here puts a foreign entry into the
    // new workspace's history — Back then opens the sibling worktree's file of
    // the same name. Adopt the incoming position (done above) and record nothing.
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
