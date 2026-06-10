import { create } from 'zustand'

// Backend connection state derived from the wsManager's channels:
// - 'idle': no channels have reported yet (nothing to say about the backend)
// - 'connected': at least one channel has an open socket
// - 'disconnected': channels exist but every socket is closed / retrying
export type ConnectionStatus = 'idle' | 'connected' | 'disconnected'

interface ConnectionState {
  status: ConnectionStatus
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
}))

function publish(): void {
  const status = deriveStatus()
  if (useConnectionStore.getState().status !== status) {
    useConnectionStore.setState({ status })
  }
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
  useConnectionStore.setState({ status: 'idle' })
}
