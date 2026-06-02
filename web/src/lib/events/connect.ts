import { useWorkspaceListStore } from '@/lib/store/workspace-list'
import type { DaemonEvent } from './types'

type EventOf<T extends DaemonEvent['type']> = Extract<DaemonEvent, { type: T }>

export function connectDaemonEvents(): () => void {
  const unlisteners: Array<() => void> = []

  unlisteners.push(
    window.__CROWBAR__.on<EventOf<'workspace:updated'>>('workspace:updated', () => {
      void useWorkspaceListStore.getState().fetch()
    }),
  )

  unlisteners.push(
    window.__CROWBAR__.on<EventOf<'git:changed'>>('git:changed', () => {
      window.dispatchEvent(new CustomEvent('git-status-changed'))
    }),
  )

  unlisteners.push(
    window.__CROWBAR__.on<EventOf<'file:changed'>>('file:changed', (p) => {
      window.dispatchEvent(new CustomEvent('file-external-change', { detail: { event_type: 'modify', path: p.path } }))
    }),
  )

  // chat:message — live chat updates arrive via the chat WebSocket stream; no refetch needed.

  return () => unlisteners.forEach(u => u())
}
