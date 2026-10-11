import { isTauri } from '@/lib/crowbar-bridge'
import { streamFailureOf } from '@/lib/api/errors'
import { splitHostPath } from '@/lib/host-path'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { getHostRuntime, subscribeHostStatus } from '@/features/hosts/stores/host-runtime-registry'
import { LOCAL_HOST } from '@/features/hosts/types'

import { wsUrl, isWebSocketCapable } from './url'
import { reportChannelState, reportChannelFailure, reportChannelGone } from './connection-store'
import { TauriWebSocket } from './tauri-transport'
import { noteDaemonChange } from '@/lib/transport/daemon-changes'

type Callback = (data: unknown) => void

// The subset of the WebSocket surface the manager drives. Both the native
// `WebSocket` and the desktop `TauriWebSocket` shim satisfy it, so the manager
// is transport-agnostic.
interface WSLike {
  onopen: (() => void) | null
  onmessage: ((event: { data: string }) => void) | null
  onclose: (() => void) | null
  onerror: ((event: unknown) => void) | null
  readyState: number
  send(data: string): void
  close(): void
  addEventListener?: (type: 'open', listener: () => void, options?: { once: boolean }) => void
}

interface Channel {
  /** Null while parked: its host may not be dialled now (see mayDial). */
  socket: WSLike | null
  callbacks: Set<Callback>
  reconnectDelay: number
  endpoint: string
  hostId: string
}

// A remote host whose status is down or needs repair refuses every attempt, and one nobody
// has paired is not to be dialled: its channels wait, parked, for it to be 'up' again. The
// local daemon is not probed; it is always dialled.
function mayDial(hostId: string): boolean {
  if (hostId === LOCAL_HOST.id) return true
  if (!useHostConfigStore.getState().hosts.some((host) => host.id === hostId)) return false
  const status = getHostRuntime(hostId)?.getState().status
  return status !== 'down' && status !== 'needs-repair'
}

// The daemon pings every stream each 30 s. A remote one silent for two pings is half-open
// (a network change, sleep): the bridge closes it, and it is redialled like any drop instead of
// read as live forever. The local daemon is a unix socket away, with no network to lose.
const REMOTE_READ_IDLE_TIMEOUT_MS = 60_000

// The live transport for the active environment: the native WebSocket in the
// browser, the unix-socket-bridged shim on desktop (where `crowbar://` has no
// native WebSocket). A host-qualified endpoint dials that host.
export function createTransport(endpoint: string): WSLike {
  const { hostId, path } = splitHostPath(endpoint)
  if (isTauri()) {
    if (hostId === LOCAL_HOST.id) return new TauriWebSocket(path)
    return new TauriWebSocket(path, { hostId, idleTimeoutMs: REMOTE_READ_IDLE_TIMEOUT_MS })
  }
  return new WebSocket(wsUrl(path, hostId)) as WSLike
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
function closeSocketQuietly(socket: WSLike): void {
  if (socket.readyState === WebSocket.CONNECTING && socket.addEventListener) {
    socket.addEventListener('open', () => socket.close(), { once: true })
  } else if (socket.readyState === WebSocket.OPEN) {
    socket.close()
  } else {
    socket.close()
  }
}

export function createWSManager(): WSManager {
  const channels = new Map<string, Channel>()
  /** Per remote host with parked channels, the watch on its status that reopens them. */
  const parkedHosts = new Map<string, () => void>()

  function park(endpoint: string, hostId: string, callbacks: Set<Callback>): Channel {
    const ch: Channel = { socket: null, callbacks, reconnectDelay: 1000, endpoint, hostId }
    channels.set(endpoint, ch)
    reportChannelState(endpoint, false, hostId)
    if (!parkedHosts.has(hostId)) {
      const watch = subscribeHostStatus(hostId, () => {
        // Up but not dialable (no longer paired) stays parked: reopening would park again.
        if (getHostRuntime(hostId)?.getState().status === 'up' && mayDial(hostId)) {
          reopenParked(hostId)
        }
      })
      parkedHosts.set(hostId, watch)
    }
    return ch
  }

  function reopenParked(hostId: string): void {
    parkedHosts.get(hostId)?.()
    parkedHosts.delete(hostId)
    for (const ch of [...channels.values()]) {
      if (ch.socket === null && ch.hostId === hostId) open(ch.endpoint, 1000, true, ch.callbacks)
    }
  }

  function stopWatchingIfNoneParked(hostId: string): void {
    for (const ch of channels.values()) if (ch.socket === null && ch.hostId === hostId) return
    parkedHosts.get(hostId)?.()
    parkedHosts.delete(hostId)
  }

  /** `resumed`: this socket replaces a dropped one, so its open is the moment
   *  the stream is back and subscribers must refetch what they missed. */
  function open(
    endpoint: string,
    reconnectDelay = 1000,
    resumed = false,
    callbacks = new Set<Callback>(),
  ): Channel {
    const { hostId } = splitHostPath(endpoint)
    if (!mayDial(hostId)) return park(endpoint, hostId, callbacks)
    const socket = createTransport(endpoint)
    const ch: Channel = { socket, callbacks, reconnectDelay, endpoint, hostId }

    // A successful connection resets the backoff so the next outage starts
    // from the base delay again.
    socket.onopen = () => {
      ch.reconnectDelay = 1000
      if (channels.get(endpoint) !== ch) return
      reportChannelState(endpoint, true, hostId)
      // Only now, not per attempt: a refetch while the daemon is still down
      // just fails, and every subscriber's refetch would repeat each retry.
      if (!resumed) return
      noteDaemonChange()
      ch.callbacks.forEach((cb) => cb({ reconnected: true }))
    }

    socket.onmessage = (e) => {
      let parsed: unknown
      try {
        parsed = JSON.parse(e.data as string)
      } catch {
        parsed = e.data
      }
      noteDaemonChange()
      ch.callbacks.forEach((cb) => cb(parsed))
    }

    socket.onclose = () => {
      // Only the live channel for this endpoint reports state — a stale socket
      // closing after a reconnect must not flag the endpoint as down.
      if (channels.get(endpoint) === ch) reportChannelState(endpoint, false, hostId)
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
        // Carry the (doubled) backoff into the new channel so repeated failures
        // actually back off; a host that refuses is parked here, not redialled.
        open(endpoint, Math.min(ch.reconnectDelay * 2, 30_000), true, ch.callbacks)
      }, ch.reconnectDelay)
    }

    // A refused remote open reaches here as its reason; without this the app
    // sees only that the stream closed, not whether the host is down or changed.
    if (hostId !== LOCAL_HOST.id) {
      socket.onerror = (err) => {
        const failure = streamFailureOf(err, hostId)
        if (failure && channels.get(endpoint) === ch) reportChannelFailure(endpoint, failure)
      }
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
            if (current.socket) closeSocketQuietly(current.socket)
            channels.delete(endpoint)
            reportChannelGone(endpoint)
            stopWatchingIfNoneParked(current.hostId)
          } else if (ch.socket) {
            closeSocketQuietly(ch.socket)
          }
        }
      }
    },

    send(endpoint, data) {
      const socket = channels.get(endpoint)?.socket
      if (socket?.readyState === WebSocket.OPEN) {
        socket.send(JSON.stringify(data))
      }
    },
  }
}

export const wsManager = createWSManager()
