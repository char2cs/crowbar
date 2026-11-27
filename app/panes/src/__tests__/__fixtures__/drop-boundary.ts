import { vi } from 'vitest'
import * as chatCommands from '@/features/agent/api/agent-chat-commands'
import { toast } from '@/features/window/stores/toast-store'
import * as homeResolver from '@/features/workspace/lib/home-workspace-resolver'
import * as placement from '@/lib/api/sidebar-placement'
import * as workspaceApi from '@/lib/api/workspace'
import * as layoutPersistence from '@/lib/persistence/workspace-layout'

const placed = <T>(row: object): T => row as T

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
