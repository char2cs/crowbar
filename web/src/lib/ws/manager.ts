import { wsUrl, isWebSocketCapable } from './url'
import { reportChannelState, reportChannelGone } from './connection-store'

type Callback = (data: unknown) => void

interface Channel {
  socket: WebSocket
  callbacks: Set<Callback>
  reconnectDelay: number
  endpoint: string
}

export interface WSManager {
  subscribe(endpoint: string, cb: Callback): () => void
  send(endpoint: string, data: unknown): void
}

// Closing a socket that is still CONNECTING makes the browser log
// "WebSocket is closed before the connection is established" (StrictMode
// mounts tear channels down before the handshake finishes). Defer the close
// until the socket opens so the console stays clean; a socket that errors
// while CONNECTING closes itself and needs no action.
function closeSocketQuietly(socket: WebSocket): void {
  if (socket.readyState === WebSocket.CONNECTING) {
    socket.addEventListener('open', () => socket.close(), { once: true })
  } else if (socket.readyState === WebSocket.OPEN) {
    socket.close()
  }
}

export function createWSManager(): WSManager {
  const channels = new Map<string, Channel>()

  function open(endpoint: string, reconnectDelay = 1000): Channel {
    const ch: Channel = {
      socket: new WebSocket(wsUrl(endpoint)),
      callbacks: new Set(),
      reconnectDelay,
      endpoint,
    }

    // A successful connection resets the backoff so the next outage starts
    // from the base delay again.
    ch.socket.onopen = () => {
      ch.reconnectDelay = 1000
      if (channels.get(endpoint) === ch) reportChannelState(endpoint, true)
    }

    ch.socket.onmessage = (e) => {
      let parsed: unknown
      try {
        parsed = JSON.parse(e.data as string)
      } catch {
        parsed = e.data
      }
      ch.callbacks.forEach((cb) => cb(parsed))
    }

    ch.socket.onclose = () => {
      // Only the live channel for this endpoint reports state — a stale socket
      // closing after a reconnect must not flag the endpoint as down.
      if (channels.get(endpoint) === ch) reportChannelState(endpoint, false)
      if (ch.callbacks.size === 0) return
      setTimeout(() => {
        // All subscribers may have left while we waited — do not resurrect a
        // socket nobody listens to.
        if (ch.callbacks.size === 0) {
          if (channels.get(endpoint) === ch) {
            channels.delete(endpoint)
            reportChannelGone(endpoint)
          }
          return
        }
        // Carry the (doubled) backoff into the new channel so repeated
        // failures actually back off instead of restarting at the base delay.
        const fresh = open(endpoint, Math.min(ch.reconnectDelay * 2, 30_000))
        fresh.callbacks = ch.callbacks
        channels.set(endpoint, fresh)
        // Tell subscribers the stream was interrupted so they can refetch
        // whatever pushes they may have missed during the outage.
        ch.callbacks.forEach((cb) => cb({ reconnected: true }))
      }, ch.reconnectDelay)
    }

    channels.set(endpoint, ch)
    return ch
  }

  return {
    subscribe(endpoint, cb) {
      // The active transport may not be able to carry a WebSocket (e.g. the
      // desktop's crowbar:// unix-socket scheme). Constructing one throws and
      // crashes startup, so skip the live channel entirely — mark it gone so
      // the UI reflects "no live stream" rather than a perpetual "connecting".
      if (!isWebSocketCapable()) {
        reportChannelGone(endpoint)
        return () => {}
      }
      const ch = channels.get(endpoint) ?? open(endpoint)
      ch.callbacks.add(cb)
      return () => {
        ch.callbacks.delete(cb)
        if (ch.callbacks.size === 0) {
          // The channel may have been replaced by a reconnect since this
          // subscription was created; close the live socket, not the stale one.
          const current = channels.get(endpoint)
          if (current && current.callbacks === ch.callbacks) {
            closeSocketQuietly(current.socket)
            channels.delete(endpoint)
            reportChannelGone(endpoint)
          } else {
            closeSocketQuietly(ch.socket)
          }
        }
      }
    },

    send(endpoint, data) {
      const ch = channels.get(endpoint)
      if (ch?.socket.readyState === WebSocket.OPEN) {
        ch.socket.send(JSON.stringify(data))
      }
    },
  }
}

export const wsManager = createWSManager()
