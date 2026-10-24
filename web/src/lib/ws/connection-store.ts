import { create } from 'zustand'

// Backend connection state derived from the wsManager's channels:
// - 'idle': no channels have reported yet (nothing to say about the backend)
// - 'connected': at least one channel has an open socket
// - 'disconnected': channels exist but every socket is closed / retrying
export type ConnectionStatus = 'idle' | 'connected' | 'disconnected'

interface ConnectionState {
  status: ConnectionStatus
  /** Connected, but some channel is closed and retrying. */
  degraded: boolean
  /** Times every channel was lost and the backend came back; the first connect is not one. */
  reconnects: number
}

// Per-endpoint open/closed flags live outside the store so the store holds a
// few primitives — components subscribe with a narrow selector on each.
const channelStates = new Map<string, boolean>()

function deriveStatus(): ConnectionStatus {
  if (channelStates.size === 0) return 'idle'
  for (const open of channelStates.values()) {
    if (open) return 'connected'
  }
  return 'disconnected'
}

function deriveDegraded(status: ConnectionStatus): boolean {
  if (status !== 'connected') return false
  for (const open of channelStates.values()) {
    if (!open) return true
  }
  return false
}

export const useConnectionStore = create<ConnectionState>(() => ({
  status: 'idle',
  degraded: false,
  reconnects: 0,
}))

function publish(): void {
  const status = deriveStatus()
  const degraded = deriveDegraded(status)
  const prev = useConnectionStore.getState()
  if (prev.status === status && prev.degraded === degraded) return
  const recovered = prev.status === 'disconnected' && status === 'connected'
  useConnectionStore.setState({
    status,
    degraded,
    reconnects: recovered ? prev.reconnects + 1 : prev.reconnects,
  })
}

/** Called by the wsManager when a channel's socket opens or closes. */
export function reportChannelState(endpoint: string, open: boolean): void {
  channelStates.set(endpoint, open)
  publish()
}

/** Called by the wsManager when a channel is torn down (last subscriber left). */
export function reportChannelGone(endpoint: string): void {
  channelStates.delete(endpoint)
  publish()
}

/** Test helper: clear all tracked channels. */
export function resetConnectionState(): void {
  channelStates.clear()
  useConnectionStore.setState({ status: 'idle', degraded: false, reconnects: 0 })
}
