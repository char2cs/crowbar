import type { StreamCallback } from './stream-callback'
import type { Waiter } from './waiter'
import type { WsLike } from './ws-like'

export interface Channel {
  socket: WsLike | null
  callbacks: Set<StreamCallback>
  opened: boolean
  reconnectDelay: number
  endpoint: string
  hostId: string
  waiters: Waiter[]
  outbox: string[]
  failure?: unknown
}
