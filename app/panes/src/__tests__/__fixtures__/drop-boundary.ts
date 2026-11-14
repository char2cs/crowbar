import { vi } from 'vitest'
import * as chatCommands from '@/features/agent/api/agent-chat-commands'
import { toast } from '@/features/window/stores/toast-store'
import * as homeResolver from '@/features/workspace/lib/home-workspace-resolver'
import * as placement from '@/lib/api/sidebar-placement'
import * as workspaceApi from '@/lib/api/workspace'
import * as layoutPersistence from '@/lib/persistence/workspace-layout'

// The envelopes are partial DTOs: a drop applies only the placed id, parent and order.
const placed = <T>(row: object): T => row as T

/** Every daemon write a sidebar drop makes, answered as the real endpoint does: a placement
 *  echoes the request back, since the drop applies the response (an empty one would pass by
 *  accident). No home workspace is resolved until a test names one. */
export function stubDropBoundary(): void {
  vi.spyOn(layoutPersistence, 'saveWorkspaceLayout').mockResolvedValue(undefined)
  vi.spyOn(toast, 'error').mockImplementation(() => '')
  vi.spyOn(toast, 'info').mockImplementation(() => '')
  vi.spyOn(toast, 'success').mockImplementation(() => '')
  vi.spyOn(placement, 'placeWorkspace').mockImplementation(async (wsId, at) =>
    placed({
      workspace: { id: wsId, parentId: at.folderId ?? '', order: at.order ?? 0 },
      shifted: [],
    }),
  )
  vi.spyOn(placement, 'placeFolder').mockImplementation(async (projectId, repoId, id, at) =>
    placed({
      folder: {
        id,
        repoId,
        projectId,
        name: id,
        parentId: at.parentId ?? '',
        order: at.order ?? 0,
      },
      shifted: [],
    }),
  )
  vi.spyOn(placement, 'placeHomeFolder').mockImplementation(async (projectId, id, at) =>
    placed({
      folder: {
        id,
        repoId: '',
        projectId,
        name: id,
        parentId: at.parentId ?? '',
        order: at.order ?? 0,
      },
      shifted: [],
    }),
  )
  // PATCH .../repos/:id answers 204: the repo's new order rides the `repos` broadcast.
  vi.spyOn(placement, 'placeRepo').mockResolvedValue(undefined)
  vi.spyOn(workspaceApi, 'reparentWorkspace').mockResolvedValue(placed({}))
  vi.spyOn(chatCommands, 'setChatPlacement').mockImplementation(async (workspaceId, id, at) =>
    placed({
      chat: { id, workspaceId, parentId: at.parentId ?? '', order: at.order ?? 0 },
      shifted: [],
    }),
  )
  vi.spyOn(homeResolver, 'getHomeWorkspaceId').mockReturnValue(null)
  vi.spyOn(homeResolver, 'getHomeOwningChatId').mockReturnValue(null)
}
