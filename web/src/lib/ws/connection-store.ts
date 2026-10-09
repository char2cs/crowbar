import { create } from 'zustand'
import type { ApiError } from '@/lib/api/errors'
import { LOCAL_HOST } from '@/features/hosts/types'

// Backend connection state derived from the wsManager's channels:
// - 'idle': no channels have reported yet (nothing to say about the backend)
// - 'connected': at least one channel has an open socket
// - 'disconnected': channels exist but every socket is closed / retrying
export type ConnectionStatus = 'idle' | 'connected' | 'disconnected'

interface ConnectionState {
  status: ConnectionStatus
  /** Why a remote host's stream last failed to open, by endpoint. Cleared when
   *  that stream opens or is torn down. */
  failures: Record<string, ApiError>
}

// Per-endpoint open/closed flags live outside the store so the store holds a
// single primitive — components subscribe with a narrow selector on `status`.
const channelStates = new Map<string, boolean>()

function deriveStatus(): ConnectionStatus {
  if (channelStates.size === 0) return 'idle'
  for (const open of channelStates.values()) {
    if (open) return 'connected'
  }
  return 'disconnected'
}

export const useConnectionStore = create<ConnectionState>(() => ({
  status: 'idle',
  failures: {},
}))

function publish(): void {
  const status = deriveStatus()
  if (useConnectionStore.getState().status !== status) {
    useConnectionStore.setState({ status })
  }
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
 *  local daemon's channels speak for the backend's status: a remote host going
 *  up or down must neither hide nor cause a local outage. */
export function reportChannelState(endpoint: string, open: boolean, hostId: string): void {
  if (open) setFailure(endpoint, undefined)
  if (hostId !== LOCAL_HOST.id) return
  channelStates.set(endpoint, open)
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
  useConnectionStore.setState({ status: 'idle', failures: {} })
}
