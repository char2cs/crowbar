import type { StreamCallback } from './stream-callback'
import type { Waiter } from './waiter'

/** A channel reopening after a drop or a park: it takes over its predecessor's subscribers and
 *  waiters, and its open is the moment the stream is back, so subscribers refetch what they missed. */
export interface Retry {
  callbacks: Set<StreamCallback>
  waiters: Waiter[]
  reconnectDelay: number
}
