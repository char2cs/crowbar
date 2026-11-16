import type { Waiter } from './waiter'

/** Rejects and drops every waiter, naming the endpoint, why, and what the transport reported. */
export function refuseWaiters(
  waiters: Waiter[],
  endpoint: string,
  why: string,
  cause?: unknown,
): void {
  for (const waiter of waiters.splice(0)) waiter.reject(new Error(`${endpoint}: ${why}`, { cause }))
}
