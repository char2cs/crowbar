import type { ChannelContext } from './channel-context'
import { isWebSocketCapable } from './url'

/** Resolves once `endpoint`'s socket is open; rejects when nothing can carry it. */
export function openedOn(ctx: ChannelContext, endpoint: string): Promise<void> {
  const ch = ctx.channels.get(endpoint)
  if (!isWebSocketCapable() || !ch) {
    return Promise.reject(new Error(`${endpoint}: no socket can carry it`))
  }
  if (ch.socket?.readyState === WebSocket.OPEN) return Promise.resolve()
  return new Promise((resolve, reject) => ch.waiters.push({ resolve, reject }))
}
