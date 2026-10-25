import { Fragment } from 'react'
import { getBuildInfo } from '@/lib/build-info'
import type { BuildChannel, BuildInfo } from '@/lib/build-info'
import { useSettingsStore } from '@/features/settings/store'
import { cn } from '@/utils/cn'
import { useFrameStats, type FrameStats } from '@/lib/frame-stats'
import { useConnectionStore } from '@/lib/ws/connection-store'
import { useTerminalBufferCount } from '@/features/workspace/stores/hooks/use-buffer-store'
import {
  useOpenWorkspaceCount,
  useWorkingChatCount,
} from '@/features/workspace/stores/hooks/use-open-workspace-counts'
import { useHostDotState } from '@/features/hosts/hooks/use-host-status'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import {
  formatHostEntry,
  formatReadout,
  hostNetStateOf,
  netStateOf,
  type HostNetState,
} from '@/components/layout/build-readout-lines'

function previewInfo(channel: BuildChannel): BuildInfo {
  const timestamp = new Date().toISOString()
  if (channel === 'nightly') return { channel, timestamp }
  if (channel === 'beta') return { channel, version: '0.0.0-beta.1', timestamp }
  if (channel === 'release') return { channel, version: '0.0.0' }
  return { channel: 'dev', timestamp }
}

/** Settings override wins; 'auto' (the default) detects the real build. */
function useResolvedBuildInfo(): BuildInfo | null {
  const override = useSettingsStore((s) => s.settings.buildBadgeOverride)
  if (override === 'off') return null
  if (!override || override === 'auto') return getBuildInfo()
  return previewInfo(override)
}

const READOUT_INK = '#E0E0D0'
const SEP = '  '
// A segment never breaks inside; the first line wraps only between segments.
const SEGMENT = 'whitespace-nowrap'

const NET_INK: Record<HostNetState, string | undefined> = {
  ok: undefined,
  idle: undefined,
  reconnecting: 'text-warning',
  repair: 'text-warning',
  down: 'text-destructive',
}

function HostEntry({ hostId, name }: { hostId: string; name: string }) {
  const net = hostNetStateOf(useHostDotState(hostId))
  return (
    <span data-host-net={net} className={cn(SEGMENT, NET_INK[net])}>
      {formatHostEntry(name, net)}
    </span>
  )
}

/** Each paired remote host's state, after Local's own net reading; a local-only install has none. */
function HostEntries() {
  const hosts = useHostConfigStore((s) => s.hosts)
  return (
    <>
      {hosts.map((host) => (
        <Fragment key={host.id}>
          {SEP}
          <HostEntry hostId={host.id} name={host.name} />
        </Fragment>
      ))}
    </>
  )
}

function Lines({ info, frame }: { info: BuildInfo; frame: FrameStats }) {
  const status = useConnectionStore((s) => s.status)
  const degraded = useConnectionStore((s) => s.degraded)
  const reconnects = useConnectionStore((s) => s.reconnects)
  const pty = useTerminalBufferCount()
  const ag = useWorkingChatCount()
  const ws = useOpenWorkspaceCount()
  const net = netStateOf(status, degraded)
  const lines = formatReadout({ info, net, reconnects, frame, counts: { pty, ag, ws } })

  return (
    <>
      <span>
        <span className={SEGMENT}>{lines.build}</span>
        {SEP}
        <span data-net-state={net} className={cn(SEGMENT, lines.netWarn && NET_INK[net])}>
          {lines.net}
        </span>
        {lines.reconnects && SEP + lines.reconnects}
        <HostEntries />
      </span>
      <span>{lines.stats}</span>
    </>
  )
}

/**
 * Build, daemon connection and frame stats, set like a net-graph: pale yellow
 * monospace with a dark outline, straight on the chrome. Purely visual: it
 * takes no layout space and no pointer events, so a tab under it stays
 * clickable. It is as wide as its text up to 42ch, then wraps between segments,
 * growing down from its anchor (`top-auto bottom-0` grows up); an anchor with a
 * real box (the sidebar foot) passes `w-auto` and both side insets.
 * The frame loop and the store watchers run only while this is shown
 * (Settings → Appearance → Build readout).
 */
export function BuildReadout({ className }: { className?: string }) {
  const info = useResolvedBuildInfo()
  const visible = useSettingsStore((s) => s.settings.showBuildReadout)
  const frame = useFrameStats(visible)
  if (!visible || !info) return null

  return (
    <div
      aria-hidden="true"
      style={{ color: READOUT_INK }}
      className={cn(
        'pointer-events-none absolute top-0 z-50 flex min-h-full w-max max-w-[42ch] select-none flex-col items-start justify-center whitespace-pre-wrap font-mono text-[10px] leading-3 tabular-nums [text-shadow:0_0_2px_#000,0_1px_1px_#000,1px_0_1px_#000]',
        className,
      )}
    >
      <Lines info={info} frame={frame} />
    </div>
  )
}
