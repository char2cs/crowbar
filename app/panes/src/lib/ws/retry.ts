import type { StreamCallback } from './stream-callback'
import type { Waiter } from './waiter'

export interface Retry {
  callbacks: Set<StreamCallback>
  waiters: Waiter[]
  reconnectDelay: number
}
