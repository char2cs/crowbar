/** The first wait before a redial, and where the backoff starts again after a success. */
export const BASE_RECONNECT_DELAY_MS = 1000
const MAX_RECONNECT_DELAY_MS = 30_000

/** The wait after `current`: doubled, so repeated failures back off, up to the cap. */
export function nextReconnectDelay(current: number): number {
  return Math.min(current * 2, MAX_RECONNECT_DELAY_MS)
}
