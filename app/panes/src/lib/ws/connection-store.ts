import { create } from 'zustand'
import type { ApiError } from '@/lib/errors/api-error'
import type { Problem } from '@/lib/errors/problem'
import { LOCAL_HOST } from '@/features/hosts/types'
import { useDaemonHaltStore } from '@/features/window/stores/daemon-halt-store'

export type ConnectionStatus = 'idle' | 'connected' | 'disconnected' | 'migrating'

export interface Migrating {
  message: string
  step?: { index: number; total: number; name: string }
}

interface ConnectionState {
  status: ConnectionStatus
  migrating: Migrating | null
  degraded: boolean
  reconnects: number
  hostStreams: Record<string, ConnectionStatus>
  failures: Record<string, ApiError>
  channels: Record<string, ChannelState>
}

interface ChannelState {
  hostId: string
  open: boolean
}

function deriveStatus(channels: Record<string, ChannelState>, hostId: string): ConnectionStatus {
  let seen = false
  for (const channel of Object.values(channels)) {
    if (channel.hostId !== hostId) continue
    if (channel.open) return 'connected'
    seen = true
  }
  return seen ? 'disconnected' : 'idle'
}

function deriveDegraded(channels: Record<string, ChannelState>, status: ConnectionStatus): boolean {
  if (status !== 'connected') return false
  for (const channel of Object.values(channels)) {
    if (channel.hostId === LOCAL_HOST.id && !channel.open) return true
  }
  return false
}

export const useConnectionStore = create<ConnectionState>(() => ({
  status: 'idle',
  migrating: null,
  degraded: false,
  reconnects: 0,
  hostStreams: {},
  failures: {},
  channels: {},
}))

function publish(migrating: Migrating | null): void {
  const prev = useConnectionStore.getState()
  const { channels } = prev
  const local = deriveStatus(channels, LOCAL_HOST.id)
  const nextMigrating = local === 'connected' ? null : migrating
  const status: ConnectionStatus = nextMigrating ? 'migrating' : local
  const degraded = deriveDegraded(channels, status)
  const hostStreams: Record<string, ConnectionStatus> = {}
  for (const { hostId } of Object.values(channels)) {
    if (hostId !== LOCAL_HOST.id) hostStreams[hostId] = deriveStatus(channels, hostId)
  }
  const sameHosts =
    Object.keys(hostStreams).length === Object.keys(prev.hostStreams).length &&
    Object.entries(hostStreams).every(([hostId, next]) => prev.hostStreams[hostId] === next)
  if (
    prev.status === status &&
    prev.migrating === nextMigrating &&
    prev.degraded === degraded &&
    sameHosts
  ) {
    return
  }
  const recovered = prev.status === 'disconnected' && status === 'connected'
  useConnectionStore.setState({
    status,
    migrating: nextMigrating,
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

export function reportChannelState(endpoint: string, open: boolean, hostId: string): void {
  if (open) setFailure(endpoint, undefined)
  const { channels } = useConnectionStore.getState()
  useConnectionStore.setState({ channels: { ...channels, [endpoint]: { hostId, open } } })
  publish(useConnectionStore.getState().migrating)
}

export function reportChannelFailure(endpoint: string, failure: ApiError): void {
  setFailure(endpoint, failure)
}

export function reportChannelGone(endpoint: string): void {
  setFailure(endpoint, undefined)
  const channels = { ...useConnectionStore.getState().channels }
  delete channels[endpoint]
  useConnectionStore.setState({ channels })
  publish(useConnectionStore.getState().migrating)
}

export function reportMigrating(migrating: Migrating): void {
  publish(migrating)
}

export function reportDaemonRefusal(problem: Problem): boolean {
  if (problem.code === 'daemon_migrating') reportMigrating({ message: problem.message })
  else if (problem.code === 'daemon_halted') useDaemonHaltStore.getState().halt(problem.message)
  else return false
  return true
}
