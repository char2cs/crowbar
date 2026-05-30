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

export function createWSManager(): WSManager {
  const channels = new Map<string, Channel>()

  function open(endpoint: string): Channel {
    const ch: Channel = {
      socket: new WebSocket(endpoint),
      callbacks: new Set(),
      reconnectDelay: 1000,
      endpoint,
    }

    ch.socket.onmessage = (e) => {
      let parsed: unknown
      try { parsed = JSON.parse(e.data as string) } catch { parsed = e.data }
      ch.callbacks.forEach(cb => cb(parsed))
    }

    ch.socket.onclose = () => {
      if (ch.callbacks.size === 0) return
      setTimeout(() => {
        const fresh = open(endpoint)
        fresh.callbacks = ch.callbacks
        channels.set(endpoint, fresh)
        ch.callbacks.forEach(cb => cb({ reconnected: true }))
      }, ch.reconnectDelay)
      ch.reconnectDelay = Math.min(ch.reconnectDelay * 2, 30_000)
    }

    channels.set(endpoint, ch)
    return ch
  }

  return {
    subscribe(endpoint, cb) {
      const ch = channels.get(endpoint) ?? open(endpoint)
      ch.callbacks.add(cb)
      return () => {
        ch.callbacks.delete(cb)
        if (ch.callbacks.size === 0) {
          ch.socket.close()
          channels.delete(endpoint)
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
