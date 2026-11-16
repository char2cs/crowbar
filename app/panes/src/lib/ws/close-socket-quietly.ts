import type { WsLike } from './ws-like'

/** Closes `socket` without the browser's "closed before the connection is established" log:
 *  StrictMode tears channels down mid-handshake, so a CONNECTING socket closes once it opens
 *  (one that errors while CONNECTING closes itself). */
export function closeSocketQuietly(socket: WsLike): void {
  if (socket.readyState === WebSocket.CONNECTING && socket.addEventListener) {
    socket.addEventListener('open', () => socket.close(), { once: true })
    return
  }
  socket.close()
}
