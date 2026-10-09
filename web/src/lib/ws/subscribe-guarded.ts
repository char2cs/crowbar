import { wsManager } from '@/lib/ws/manager'

/**
 * `wsManager.subscribe` for an effect. An endpoint that cannot be built or opened
 * (a malformed host prefix, a remote host in a plain browser) means no live channel,
 * never an effect that throws into the error boundary.
 */
export function subscribeGuarded(
  endpoint: () => string,
  onFrame: (data: unknown) => void,
): () => void {
  try {
    return wsManager.subscribe(endpoint(), onFrame)
  } catch (err) {
    console.error('[ws] no live channel:', err)
    return () => {}
  }
}
