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
  /** The same derivation per remote host, from that host's own channels. */
  hostStatus: Record<string, ConnectionStatus>
  /** Why a remote host's stream last failed to open, by endpoint. Cleared when
   *  that stream opens or is torn down. */
  failures: Record<string, ApiError>
}

// Per-endpoint open/closed flags live outside the store so the store holds
// primitives — components subscribe with a narrow selector on `status`.
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

export const useConnectionStore = create<ConnectionState>(() => ({
  status: 'idle',
  hostStatus: {},
  failures: {},
}))

function publish(): void {
  const state = useConnectionStore.getState()
  const status = deriveStatus(LOCAL_HOST.id)
  const hostStatus: Record<string, ConnectionStatus> = {}
  for (const { hostId } of channelStates.values()) {
    if (hostId !== LOCAL_HOST.id) hostStatus[hostId] = deriveStatus(hostId)
  }
  const sameHosts =
    Object.keys(hostStatus).length === Object.keys(state.hostStatus).length &&
    Object.entries(hostStatus).every(([hostId, next]) => state.hostStatus[hostId] === next)
  if (state.status === status && sameHosts) return
  useConnectionStore.setState({ status, hostStatus })
}

function statusOfHost(state: ConnectionState, hostId: string): ConnectionStatus {
  return hostId === LOCAL_HOST.id ? state.status : (state.hostStatus[hostId] ?? 'idle')
}

/** Calls `onConnected` each time `hostId`'s channels go from none open to one open
 *  ('local' follows the local backend). A caller that could not reach the host
 *  waits on this for the app's own streams to report it back. */
export function onHostConnected(hostId: string, onConnected: () => void): () => void {
  return useConnectionStore.subscribe((state, prev) => {
    if (statusOfHost(state, hostId) === 'connected' && statusOfHost(prev, hostId) !== 'connected') {
      onConnected()
    }
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
 *  must neither hide nor cause a local outage, so its own lands in `hostStatus`. */
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
  useConnectionStore.setState({ status: 'idle', hostStatus: {}, failures: {} })
}
