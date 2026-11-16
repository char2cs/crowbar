import type { ChannelContext } from './channel-context'

/** Sends `data` as JSON now on an open socket, or once it opens on a connecting one; a closed or
 *  parked channel drops it. */
export function sendOn(ctx: ChannelContext, endpoint: string, data: unknown): void {
  const ch = ctx.channels.get(endpoint)
  if (!ch?.socket) return
  const frame = JSON.stringify(data)
  if (ch.socket.readyState === WebSocket.OPEN) ch.socket.send(frame)
  if (ch.socket.readyState === WebSocket.CONNECTING) ch.outbox.push(frame)
}
