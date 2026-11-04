import { isTauri } from '@/lib/crowbar-bridge'
import { streamFailureOf } from '@/lib/api/errors'
import { splitHostPath } from '@/lib/host-path'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { getHostRuntime, subscribeHostStatus } from '@/features/hosts/stores/host-runtime-registry'
import { LOCAL_HOST } from '@/features/hosts/types'
import { hasLocalDaemon } from '@/features/hosts/lib/local-daemon'

import { wsUrl, isWebSocketCapable } from './url'
import { reportChannelState, reportChannelFailure, reportChannelGone } from './connection-store'
import { TauriWebSocket } from './tauri-transport'
import { noteSocketOpened, probeDaemon } from './daemon-probe'
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

interface Waiter {
  resolve: () => void
  reject: (reason: Error) => void
}

interface Channel {
  /** Null while parked: its host may not be dialled now (see mayDial). */
  socket: WSLike | null
  callbacks: Set<Callback>
  /** Its socket's open has happened and it has not closed since. */
  opened: boolean
  reconnectDelay: number
  endpoint: string
  hostId: string
  /** Waiting on this socket's open; once it has closed, on the reconnect's, which takes them. */
  waiters: Waiter[]
  /** Sent while this socket connected; dies with it, as a reconnect's subscribers ask afresh. */
  outbox: string[]
  /** What the transport reported failing, handed to the waiters it refuses. */
  failure?: unknown
}

/** What a reopened channel takes over from the one it replaces. */
interface Carried {
  callbacks: Set<Callback>
  waiters: Waiter[]
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

function isLocal(endpoint: string): boolean {
  return splitHostPath(endpoint).hostId === LOCAL_HOST.id
}

export interface WSManager {
  subscribe(endpoint: string, cb: Callback): () => void
  send(endpoint: string, data: unknown): void
  /**
   * Resolves once `endpoint`'s socket is open, at once if it is. Rejects, with the
   * transport's failure as its cause, when the one attempt it waits on closes first:
   * the one in flight, or the reconnect a dropped socket makes after its backoff
   * (≤30s). A permanent refusal is not told apart; a caller wanting longer awaits again.
   */
  opened(endpoint: string): Promise<void>
  /** `listener` hears each drop of `endpoint`'s open socket while it has subscribers. */
  onDrop(endpoint: string, listener: () => void): () => void
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

function refuse(waiters: Waiter[], endpoint: string, why: string, cause?: unknown): void {
  for (const waiter of waiters.splice(0)) waiter.reject(new Error(`${endpoint}: ${why}`, { cause }))
}

export function createWSManager(): WSManager {
  const channels = new Map<string, Channel>()
  const dropListeners = new Map<string, Set<() => void>>()
  /** Per remote host with parked channels, the watch on its status that reopens them. */
  const parkedHosts = new Map<string, () => void>()

  function park(endpoint: string, hostId: string, carried: Carried): Channel {
    const ch: Channel = {
      socket: null,
      ...carried,
      opened: false,
      reconnectDelay: 1000,
      endpoint,
      hostId,
      outbox: [],
    }
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
      if (ch.socket !== null || ch.hostId !== hostId) continue
      open(ch.endpoint, 1000, true, { callbacks: ch.callbacks, waiters: ch.waiters.splice(0) })
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
    carried: Carried = { callbacks: new Set(), waiters: [] },
  ): Channel {
    const { hostId } = splitHostPath(endpoint)
    if (!mayDial(hostId)) return park(endpoint, hostId, carried)
    const socket = createTransport(endpoint)
    const ch: Channel = {
      socket,
      ...carried,
      opened: false,
      reconnectDelay,
      endpoint,
      hostId,
      outbox: [],
    }

    // A successful connection resets the backoff so the next outage starts
    // from the base delay again.
    socket.onopen = () => {
      ch.reconnectDelay = 1000
      if (channels.get(endpoint) !== ch) return
      ch.opened = true
      for (const data of ch.outbox.splice(0)) socket.send(data)
      if (hostId === LOCAL_HOST.id) noteSocketOpened()
      reportChannelState(endpoint, true, hostId)
      for (const waiter of ch.waiters.splice(0)) waiter.resolve()
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

    socket.onerror = (failure) => {
      ch.failure = failure
      // A refused remote open names why: whether the host is down or changed.
      if (hostId === LOCAL_HOST.id) return
      const reason = streamFailureOf(failure, hostId)
      if (reason && channels.get(endpoint) === ch) reportChannelFailure(endpoint, reason)
    }

    socket.onclose = () => {
      const wasOpen = ch.opened
      ch.opened = false
      // Only the live channel for this endpoint reports state — a stale socket
      // closing after a reconnect must not flag the endpoint as down.
      const live = channels.get(endpoint) === ch
      if (live) reportChannelState(endpoint, false, hostId)
      // Refused before it opened: the local daemon may be up but not serving, and
      // only its health route says why (a dropped open socket is just a drop).
      if (live && !wasOpen && hostId === LOCAL_HOST.id) void probeDaemon()
      refuse(ch.waiters, endpoint, 'closed before it opened', ch.failure)
      if (ch.callbacks.size === 0) return
      if (live && wasOpen)
        for (const listener of [...(dropListeners.get(endpoint) ?? [])]) listener()
      setTimeout(() => {
        // All subscribers may have left while we waited — do not resurrect a
        // socket nobody listens to.
        if (ch.callbacks.size === 0) {
          if (channels.get(endpoint) === ch) {
            channels.delete(endpoint)
            reportChannelGone(endpoint)
          }
          refuse(ch.waiters, endpoint, 'no subscriber left')
          return
        }
        // Carry the (doubled) backoff into the new channel so repeated failures
        // actually back off; a host that refuses is parked here, not redialled.
        open(endpoint, Math.min(ch.reconnectDelay * 2, 30_000), true, {
          callbacks: ch.callbacks,
          waiters: ch.waiters.splice(0),
        })
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
      // Nor is there a stream from a local daemon the install does not have.
      if (!isWebSocketCapable() || (!hasLocalDaemon() && isLocal(endpoint))) {
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
            refuse(current.waiters, endpoint, 'no subscriber left')
            stopWatchingIfNoneParked(current.hostId)
          } else if (ch.socket) {
            closeSocketQuietly(ch.socket)
          }
        }
      }
    },

    send(endpoint, data) {
      const ch = channels.get(endpoint)
      if (!ch?.socket) return
      if (ch.socket.readyState === WebSocket.OPEN) ch.socket.send(JSON.stringify(data))
      else if (ch.socket.readyState === WebSocket.CONNECTING) ch.outbox.push(JSON.stringify(data))
    },

    opened(endpoint) {
      const ch = channels.get(endpoint)
      if (!isWebSocketCapable() || !ch) {
        return Promise.reject(new Error(`${endpoint}: no socket can carry it`))
      }
      if (ch.socket?.readyState === WebSocket.OPEN) return Promise.resolve()
      return new Promise((resolve, reject) => ch.waiters.push({ resolve, reject }))
    },

    onDrop(endpoint, listener) {
      const listeners = dropListeners.get(endpoint) ?? new Set()
      dropListeners.set(endpoint, listeners.add(listener))
      return () => {
        listeners.delete(listener)
        if (listeners.size === 0 && dropListeners.get(endpoint) === listeners) {
          dropListeners.delete(endpoint)
        }
      }
    },
  }
}

export const wsManager = createWSManager()
