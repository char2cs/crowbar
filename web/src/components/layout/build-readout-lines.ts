import type { HostDotState } from '@/features/hosts/types'
import type { BuildInfo } from '@/lib/build-info'
import type { FrameStats } from '@/lib/frame-stats'
import type { ConnectionStatus } from '@/lib/ws/connection-store'

/** `idle`: no channel has reported yet, so there is nothing to say. */
export type NetState = 'ok' | 'reconnecting' | 'down' | 'idle'

/** A remote host's net state: a host can also refuse this client until paired again. */
export type HostNetState = NetState | 'repair'

export interface ReadoutSnapshot {
  info: BuildInfo
  net: NetState
  reconnects: number
  frame: FrameStats
  counts: { pty: number; ag: number; ws: number }
}

export interface ReadoutLines {
  build: string
  net: string
  /** `''` until the first reconnect. */
  reconnects: string
  stats: string
  netWarn: boolean
}

export function netStateOf(status: ConnectionStatus, degraded: boolean): NetState {
  if (status === 'idle') return 'idle'
  if (status === 'disconnected') return 'down'
  return degraded ? 'reconnecting' : 'ok'
}

const NET_LABEL: Record<HostNetState, string> = {
  ok: 'ok',
  reconnecting: 'reconnecting',
  down: 'DOWN',
  repair: 'repair',
  idle: '--',
}

// Answering with every live stream closed is a remote host's form of reconnecting.
const HOST_NET: Record<HostDotState, HostNetState> = {
  up: 'ok',
  'no-live-stream': 'reconnecting',
  down: 'down',
  'not-available': 'down',
  'needs-repair': 'repair',
  unknown: 'idle',
  probing: 'idle',
}

export function hostNetStateOf(state: HostDotState): HostNetState {
  return HOST_NET[state]
}

const HOST_NAME_MAX = 14

/** One remote host in the readout's net segment; entries are joined by two spaces by the caller. */
export function formatHostEntry(name: string, net: HostNetState): string {
  const short = name.length > HOST_NAME_MAX ? `${name.slice(0, HOST_NAME_MAX - 1)}…` : name
  return `${short} ${NET_LABEL[net]}`
}

function formatTimestamp(iso?: string): string {
  if (!iso) return ''
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`
}

function formatBuild(info: BuildInfo): string {
  return [
    info.channel === 'release' ? '' : info.channel,
    info.version ?? formatTimestamp(info.timestamp),
  ]
    .filter(Boolean)
    .join(' ')
}

function formatFrame({ fps, maxDt, drops }: FrameStats): string {
  const max = maxDt > 0 ? `${maxDt}ms` : '--ms'
  return `${fps > 0 ? fps : '--'}fps ${max}${drops > 0 ? `/${drops}` : ''}`
}

/** Segments within a line are joined by two spaces by the caller. */
export function formatReadout(s: ReadoutSnapshot): ReadoutLines {
  const frame = formatFrame(s.frame)
  const { pty, ag, ws } = s.counts
  return {
    build: formatBuild(s.info),
    net: `net ${NET_LABEL[s.net]}`,
    reconnects: s.reconnects > 0 ? `rc${s.reconnects}` : '',
    // While DOWN nothing refreshes the counts, so they would be a guess.
    stats: s.net === 'down' ? frame : `${frame}  pty${pty} ag${ag} ws${ws}`,
    netWarn: s.net === 'reconnecting' || s.net === 'down',
  }
}
