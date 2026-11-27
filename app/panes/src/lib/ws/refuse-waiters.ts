import type { Waiter } from './waiter'

export function refuseWaiters(
  waiters: Waiter[],
  endpoint: string,
  why: string,
  cause?: unknown,
): void {
  for (const waiter of waiters.splice(0)) waiter.reject(new Error(`${endpoint}: ${why}`, { cause }))
}
