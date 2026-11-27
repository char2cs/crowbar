import type { ChannelContext } from './channel-context'

export function sendOn(ctx: ChannelContext, endpoint: string, data: unknown): void {
  const ch = ctx.channels.get(endpoint)
  if (!ch?.socket) return
  const frame = JSON.stringify(data)
  if (ch.socket.readyState === WebSocket.OPEN) ch.socket.send(frame)
  if (ch.socket.readyState === WebSocket.CONNECTING) ch.outbox.push(frame)
}
