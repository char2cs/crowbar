import type { StreamCallback } from './stream-callback'
import type { Waiter } from './waiter'
import type { WsLike } from './ws-like'

/** One endpoint's socket and everything that rides on it. */
export interface Channel {
  /** Null while parked: its host may not be dialled now. */
  socket: WsLike | null
  callbacks: Set<StreamCallback>
  /** Its socket's open has happened and it has not closed since. */
  opened: boolean
  reconnectDelay: number
  endpoint: string
  hostId: string
  /** Waiting on this socket's open; once it has closed, on the reconnect's, which takes them. */
  waiters: Waiter[]
  /** Sent while this socket connected; dies with it, as a reconnect's subscribers ask afresh. */
  outbox: string[]
  /** What the transport reported failing, handed to the waiters it refuses. */
  failure?: unknown
}
