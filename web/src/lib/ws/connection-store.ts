import { create } from 'zustand'
import type { ApiError } from '@/lib/api/errors'
import { LOCAL_HOST } from '@/features/hosts/types'

// Backend connection state derived from the wsManager's channels:
// - 'idle': no channels have reported yet (nothing to say about the backend)
// - 'connected': at least one channel has an open socket
// - 'disconnected': channels exist but every socket is closed / retrying
export type ConnectionStatus = 'idle' | 'connected' | 'disconnected'

interface ConnectionState {
  /** The local daemon's channels only. */
  status: ConnectionStatus
  /** Connected, but some local channel is closed and retrying. */
  degraded: boolean
  /** Times every local channel was lost and the daemon came back; the first connect is not one. */
  reconnects: number
  /** The same derivation per remote host, from that host's own channels: whether what the
   *  app shows of it is kept live — not whether it is up, which its status says. */
  hostStreams: Record<string, ConnectionStatus>
  /** Why a remote host's stream last failed to open, by endpoint. Cleared when
   *  that stream opens or is torn down. */
  failures: Record<string, ApiError>
}

// Per-endpoint open/closed flags live outside the store so the store holds a
// few primitives — components subscribe with a narrow selector on each.
const channelStates = new Map<string, { hostId: string; open: boolean }>()

function deriveStatus(hostId: string): ConnectionStatus {
  let seen = false
  for (const channel of channelStates.values()) {
    if (channel.hostId !== hostId) continue
    if (channel.open) return 'connected'
    seen = true
  }
  return seen ? 'disconnected' : 'idle'
}

function deriveDegraded(status: ConnectionStatus): boolean {
  if (status !== 'connected') return false
  for (const channel of channelStates.values()) {
    if (channel.hostId === LOCAL_HOST.id && !channel.open) return true
  }
  return false
}

export const useConnectionStore = create<ConnectionState>(() => ({
  status: 'idle',
  degraded: false,
  reconnects: 0,
  hostStreams: {},
  failures: {},
}))

function publish(): void {
  const prev = useConnectionStore.getState()
  const status = deriveStatus(LOCAL_HOST.id)
  const degraded = deriveDegraded(status)
  const hostStreams: Record<string, ConnectionStatus> = {}
  for (const { hostId } of channelStates.values()) {
    if (hostId !== LOCAL_HOST.id) hostStreams[hostId] = deriveStatus(hostId)
  }
  const sameHosts =
    Object.keys(hostStreams).length === Object.keys(prev.hostStreams).length &&
    Object.entries(hostStreams).every(([hostId, next]) => prev.hostStreams[hostId] === next)
  if (prev.status === status && prev.degraded === degraded && sameHosts) return
  const recovered = prev.status === 'disconnected' && status === 'connected'
  useConnectionStore.setState({
    status,
    degraded,
    reconnects: recovered ? prev.reconnects + 1 : prev.reconnects,
    hostStreams,
  })
}

function setFailure(endpoint: string, failure: ApiError | undefined): void {
  const { failures } = useConnectionStore.getState()
  if (failures[endpoint] === failure) return
  const next = { ...failures }
  if (failure) next[endpoint] = failure
  else delete next[endpoint]
  useConnectionStore.setState({ failures: next })
}

/** Called by the wsManager when a channel's socket opens or closes. Only the
 *  local daemon's channels speak for `status`: a remote host going up or down
 *  must neither hide nor cause a local outage, so its own lands in `hostStreams`. */
export function reportChannelState(endpoint: string, open: boolean, hostId: string): void {
  if (open) setFailure(endpoint, undefined)
  channelStates.set(endpoint, { hostId, open })
  publish()
}

/** Called by the wsManager when a remote host refused a channel's open. */
export function reportChannelFailure(endpoint: string, failure: ApiError): void {
  setFailure(endpoint, failure)
}

/** Called by the wsManager when a channel is torn down (last subscriber left). */
export function reportChannelGone(endpoint: string): void {
  setFailure(endpoint, undefined)
  channelStates.delete(endpoint)
  publish()
}

/** Test helper: clear all tracked channels. */
export function resetConnectionState(): void {
  channelStates.clear()
  useConnectionStore.setState({
    status: 'idle',
    degraded: false,
    reconnects: 0,
    hostStreams: {},
    failures: {},
  })
}
