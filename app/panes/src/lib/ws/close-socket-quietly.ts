import type { WsLike } from './ws-like'

export function closeSocketQuietly(socket: WsLike): void {
  if (socket.readyState === WebSocket.CONNECTING && socket.addEventListener) {
    socket.addEventListener('open', () => socket.close(), { once: true })
    return
  }
  socket.close()
}
