import { QueryClient } from '@tanstack/react-query'
import { queryKeys } from '@/lib/queries/keys'

export function connectDaemonEvents(qc: QueryClient): void {
  window.__CROWBAR__.on('workspace:updated', () =>
    qc.invalidateQueries({ queryKey: queryKeys.workspaces.all })
  )

  window.__CROWBAR__.on<{ chatId: string }>('chat:message', (p) =>
    qc.invalidateQueries({ queryKey: queryKeys.chats.messages(p.chatId) })
  )

  window.__CROWBAR__.on<{ workspaceId: string }>('git:changed', (p) =>
    qc.invalidateQueries({ queryKey: queryKeys.git.status(p.workspaceId) })
  )

  window.__CROWBAR__.on<{ workspaceId: string; path: string }>('file:changed', (p) =>
    qc.invalidateQueries({ queryKey: queryKeys.files.tree(p.workspaceId, p.path) })
  )
}
