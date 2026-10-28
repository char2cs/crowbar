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
import { runOperation, whenSettled } from '@/lib/store/run-operation'
import { report } from '@/lib/errors/report'
import { chatNotLoadedYet } from '@/components/sidebar/lib/row-actions'
import {
  placeWorkspace,
  placeFolder,
  placeHomeFolder,
  placeRepo,
} from '@/lib/api/sidebar-placement'
import { reparentWorkspace } from '@/lib/api/workspace'
import { setChatPlacement } from '@/features/agent/api/agent-chat-commands'
import {
  refreshRepoPlacements,
  applyHomeChatPlacement,
  applyHomeFolders,
  resolveHomeRowScope,
} from '@/lib/store/home-tree'
import { toSidebarFolder } from '@/lib/store/build-repo-tree'
import { planRowDrop, UNSUPPORTED } from '@/components/sidebar/lib/drop-plan'
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
      await applyWorkspacePlacement(call.wsId, workspace, shifted)
      return
    }
    case 'folder': {
      // Applied directly, same as row-actions.ts's folder writes: there is no
      // dedicated push channel for folders any more (Task 34), so this
      // response is the only confirmation the drop gets — written through to
      // the cache every rebuild reads before the store (applied-placement.ts).
      const { folder, shifted, shiftedRows } = await placeFolder(
        call.projectId,
        call.repoId,
        call.folderId,
        { ...(call.parentId !== undefined && { parentId: call.parentId }), order: call.order },
      )
      await applyFolderPlacements(call.repoId, [folder, ...shifted], shiftedRows)
      return
    }
    case 'homeFolder': {
      // {@link applyHomeFolders}, same direct-apply reasoning as the repo
      // `'folder'` case above — home has no dedicated folder push channel
      // either. No `bump`/`crowbar_folders` write to mirror: `useHomeTreeStore`
      // is its own cache, already updated by the apply itself.
      const { folder, shifted } = await placeHomeFolder(call.projectId, call.folderId, {
        ...(call.parentId !== undefined && { parentId: call.parentId }),
        order: call.order,
      })
      applyHomeFolders(call.projectId, [folder, ...shifted].map(toSidebarFolder))
      return
    }
    case 'chat': {
      // Reported live as "can't parent a chat into a folder": the request
      // succeeded every time (confirmed live — the daemon had the chat under
      // its new parent, survived a reload), but nothing on screen ever
      // moved. `setChatPlacement`'s own response used to be discarded here,
      // same shape the folder case above already fixed for exactly this
      // reason (Task 34: no dedicated push channel) — a chat reparent turns
      // out to be the same story: whatever broadcast this was meant to ride
      // does not confirm a folder-nested move in practice, so the row just
      // sat wherever it started until an unrelated full reseed happened to
      // catch it up. Applied directly now, the same "already the daemon's
      // own committed state, arriving over the request instead of a
      // stream" reasoning `performRenameFolder` documents for its own case.
      const { chat, shifted } = await setChatPlacement(call.workspaceId, call.chatId, {
        parentId: call.parentId,
        order: call.order,
      })
      // A home chat lives in `useHomeTreeStore`, never in a repo: its own
      // response is applied there (the reseed its frame triggers is the
      // backstop); every other chat writes through the entity cache first.
      const home = resolveHomeRowScope(chat.id)
      if (home) {
        applyHomeChatPlacement(home.projectId, chat, shifted)
        return
      }
      await applyChatPlacement(chat, shifted)
      return
    }
    case 'repoHome':
      // C7: no optimistic write. The PATCH answers 204, so the order shown is
      // the daemon's own answer, read straight back (the one placement
      // writer, `applyRepoPlacements`) — a refused move never paints, so
      // there is nothing to revert, and collateral shifts to sibling repos
      // land with it.
      await placeRepo(call.projectId, call.repoId, { folderId: call.folderId, order: call.order })
      await refreshRepoPlacements(call.projectId)
      return
  }
}

/**
 * Commit a row-to-row drop — reorder/reparent a workspace or folder, place a
 * chat within its own workspace's chat tree, or — when the drop TARGET lives
 * in a Recents band (`targetInRecents`, threaded from `use-sidebar-drag.ts`'s
 * hit test) — open into that view or reorder the band itself
 * (`performRecentsDrop`, spec §8.1). `SIDEBAR_DROP_POLICY` has already
 * refused every other subject/target/mode combination, so this only ever has
 * to plan what it allows through.
 *
 * Calls fire in order, each awaited before the next: a multi-row move's
 * `order` is an index into the destination as it stands once the previous
 * call has landed, so firing them concurrently (`Promise.all`) could land
 * the move in an arbitrary arrangement (matches `sidebar-placement.ts`'s own
 * documented contract for `order`). No optimistic paint — the WS-driven
 * cache applies the daemon's confirmed state, same as every other row action
 * in this plan (`performRenameWorkspaceBranch`, `performCreateFolder`).
 */
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
    if (plan === UNSUPPORTED) {
      toast.error('Moving a chat to a different workspace is not supported yet')
      return
    }
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
