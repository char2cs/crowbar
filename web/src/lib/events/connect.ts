import { QueryClient } from '@tanstack/react-query'
import { queryKeys } from '@/lib/queries/keys'
import type { DaemonEvent } from './types'

type EventOf<T extends DaemonEvent['type']> = Extract<DaemonEvent, { type: T }>

export function connectDaemonEvents(qc: QueryClient): () => void {
  const unlisteners: Array<() => void> = []

  unlisteners.push(
    window.__CROWBAR__.on<EventOf<'workspace:updated'>>('workspace:updated', () =>
      qc.invalidateQueries({ queryKey: queryKeys.workspaces.all })
    )
  )

  unlisteners.push(
    window.__CROWBAR__.on<EventOf<'chat:message'>>('chat:message', (p) =>
      qc.invalidateQueries({ queryKey: queryKeys.chats.messages(p.chatId) })
    )
  )

  unlisteners.push(
    window.__CROWBAR__.on<EventOf<'git:changed'>>('git:changed', (p) =>
      qc.invalidateQueries({ queryKey: queryKeys.git.status(p.workspaceId) })
    )
  )

  unlisteners.push(
    window.__CROWBAR__.on<EventOf<'file:changed'>>('file:changed', (p) =>
      qc.invalidateQueries({ queryKey: queryKeys.files.tree(p.workspaceId, p.path) })
    )
  )

  return () => unlisteners.forEach(u => u())
}
