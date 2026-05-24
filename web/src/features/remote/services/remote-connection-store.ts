// Stub
import { create } from "zustand"

interface RemoteConnectionState {
  isConnected: boolean
  connections: unknown[]
  activeConnection: null
}

export const useRemoteConnectionStore = create<RemoteConnectionState>(() => ({
  isConnected: false,
  connections: [],
  activeConnection: null,
}))

export const connectionStore = useRemoteConnectionStore
