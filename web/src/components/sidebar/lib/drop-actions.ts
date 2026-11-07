import { toast } from '@/features/window/stores/toast-store'
import { type DropMode } from '@/components/tree-dnd/drop-core'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'
import {
  applyChatPlacement,
  applyFolderPlacements,
  applyWorkspacePlacement,
} from '@/lib/store/applied-placement'
import { OwningChatNotRecordedError } from '@/lib/workspace-scope-url'
import { getOwningChatId } from '@/lib/workspace-scope'
import { runOperation, whenSettled } from '@/core/entities/run-operation'
import { report } from '@/lib/errors/report'
import { hostIdForChat, hostIdForProject, hostIdForWorkspace } from '@/lib/host-resolve'
import { chatNotLoadedYet } from '@/components/sidebar/lib/row-actions'
import {
  placeWorkspace,
  placeFolder,
  placeHomeFolder,
  placeRepo,
} from '@/lib/api/sidebar-placement'
import { reparentWorkspace } from '@/lib/api/workspace'
import { cancelChatMove, setChatPlacement } from '@/features/agent/api/agent-chat-commands'
import { planRowDrop } from '@/components/sidebar/lib/drop-plan'
import type { RowPlacementCall } from '@/components/sidebar/lib/drop-plan-tree'
import { performRecentsDrop } from '@/components/sidebar/lib/pane-drop-actions'

async function fireRowPlacementCall(call: RowPlacementCall): Promise<void> {
  switch (call.kind) {
    case 'reparent': {
      // A fork whose owner is not recorded is still loading: refused before any operation.
      if (!getOwningChatId(call.wsId)) throw new OwningChatNotRecordedError(call.wsId)
      // The daemon publishes the terminal frame after the move is written, so
      // the placement below follows a landed reparent; a refusal rejects here.
      const op = runOperation(
        {
          kind: 'workspace.reparent',
          subject: { kind: 'workspace', id: call.wsId },
          projectId: call.projectId,
          label: 'Move',
        },
        (opId) => reparentWorkspace(call.wsId, call.parentId, opId),
      )
      await whenSettled(op.id)
      return
    }
    case 'workspace': {
      // A branch row's order is read off its WorkspaceDTO, which no placement
      // frame refreshes — the answer (the moved row and every shifted
      // folder/branch sibling) is applied directly.
      const { workspace, shifted } = await placeWorkspace(call.wsId, {
        ...(call.folderId !== undefined && { folderId: call.folderId }),
        order: call.order,
      })
      await applyWorkspacePlacement(hostIdForWorkspace(call.wsId), call.wsId, workspace, shifted)
      return
    }
    case 'folder': {
      const { folder, shifted, shiftedRows } = await placeFolder(
        call.projectId,
        call.repoId,
        call.folderId,
        { ...(call.parentId !== undefined && { parentId: call.parentId }), order: call.order },
      )
      await applyFolderPlacements(
        hostIdForProject(call.projectId),
        [folder, ...shifted],
        shiftedRows,
      )
      return
    }
    case 'homeFolder': {
      const { folder, shifted } = await placeHomeFolder(call.projectId, call.folderId, {
        ...(call.parentId !== undefined && { parentId: call.parentId }),
        order: call.order,
      })
      await applyFolderPlacements(hostIdForProject(call.projectId), [folder, ...shifted])
      return
    }
    case 'chat': {
      const { chat, shifted } = await setChatPlacement(call.workspaceId, call.chatId, {
        parentId: call.parentId,
        order: call.order,
        ...(call.moveTo && { workspaceId: call.moveTo.workspaceId, label: call.moveTo.label }),
      })
      await applyChatPlacement(hostIdForChat(call.chatId), chat, shifted)
      return
    }
    case 'repoHome':
      // No optimistic write: the order shown is the daemon's, as the repo frames carry it,
      // so a refused move never paints and sibling shifts land with it.
      await placeRepo(call.projectId, call.repoId, { folderId: call.folderId, order: call.order })
      return
  }
}

// Commit a row-to-row drop the policy allowed: a tree placement, or for a Recents target, open
// into or reorder the band. Calls run in sequence: each `order` indexes the destination as the
// previous call left it, so concurrent calls could land any arrangement.
export async function performSidebarDrop(
  subjects: SidebarRow[],
  target: SidebarRow,
  mode: DropMode,
  targetInRecents = false,
): Promise<void> {
  if (targetInRecents) {
    performRecentsDrop(subjects, target, mode)
    return
  }
  try {
    const plan = planRowDrop(subjects, target, mode)
    for (const call of plan) {
      // react-doctor-disable-next-line async-await-in-loop -- accepted: sequential on purpose (see this function's own doc above) — Promise.all would race the writes.
      await fireRowPlacementCall(call)
    }
  } catch (err) {
    // A reparent is chat-addressed; a fork whose owner is not recorded yet
    // is "still loading", the same copy every other verb uses (row-actions.ts).
    if (err instanceof OwningChatNotRecordedError) {
      const row = subjects.find((s) => s.workspaceId === err.wsId)
      toast.error(chatNotLoadedYet('move', row?.branchName ?? row?.label))
      return
    }
    report(err, { action: 'Move' })
  }
}

/** Withdraws the move a chat dropped into another workspace is waiting to make. */
export async function performCancelMove(row: SidebarRow): Promise<void> {
  if (!row.workspaceId) return
  try {
    await cancelChatMove(row.workspaceId, row.id)
  } catch (err) {
    report(err, { action: 'Cancel move' })
  }
}
