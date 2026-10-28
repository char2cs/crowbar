import { create } from 'zustand'
import type { Problem } from '@/lib/errors/problem'
import { useDaemonHaltStore } from '@/features/window/stores/daemon-halt-store'

// Backend connection state derived from the wsManager's channels:
// - 'idle': no channels have reported yet (nothing to say about the backend)
// - 'connected': at least one channel has an open socket
// - 'migrating': no socket is open and the daemon said it is preparing its home
// - 'disconnected': channels exist but every socket is closed / retrying
// A halted daemon is not a connection status: the daemon-halt store owns it.
export type ConnectionStatus = 'idle' | 'connected' | 'disconnected' | 'migrating'

/** What a daemon that is not serving yet says it is doing. */
export interface Migrating {
  message: string
  step?: { index: number; total: number; name: string }
}

interface ConnectionState {
  status: ConnectionStatus
  migrating: Migrating | null
}

// Per-endpoint open/closed flags live outside the store so the store holds
// only what components render — they subscribe with narrow selectors.
const channelStates = new Map<string, boolean>()

function anyOpen(): boolean {
  for (const open of channelStates.values()) {
    if (open) return true
  }
  return false
}

export const useConnectionStore = create<ConnectionState>(() => ({
  status: 'idle',
  migrating: null,
}))

function publish(migrating: Migrating | null): void {
  const open = anyOpen()
  const next = open ? null : migrating
  let status: ConnectionStatus = 'idle'
  if (open) status = 'connected'
  else if (next) status = 'migrating'
  else if (channelStates.size > 0) status = 'disconnected'
  const current = useConnectionStore.getState()
  if (current.status !== status || current.migrating !== next) {
    useConnectionStore.setState({ status, migrating: next })
  }
}

/** Called by the wsManager when a channel's socket opens or closes. */
export function reportChannelState(endpoint: string, open: boolean): void {
  channelStates.set(endpoint, open)
  publish(useConnectionStore.getState().migrating)
}

/** Called by the wsManager when a channel is torn down (last subscriber left). */
export function reportChannelGone(endpoint: string): void {
  channelStates.delete(endpoint)
  publish(useConnectionStore.getState().migrating)
}

/** The daemon is preparing its home. An open socket outranks it: that report is stale. */
export function reportMigrating(migrating: Migrating): void {
  publish(migrating)
}

/** Routes a refusal that names the daemon's own state to that state's owner. */
export function reportDaemonRefusal(problem: Problem): void {
  if (problem.code === 'daemon_migrating') reportMigrating({ message: problem.message })
  if (problem.code === 'daemon_halted') useDaemonHaltStore.getState().halt(problem.message)
}

/** Test helper: clear all tracked channels. */
export function resetConnectionState(): void {
  channelStates.clear()
  useConnectionStore.setState({ status: 'idle', migrating: null })
}
