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
import { formatReadout, netStateOf, type NetState } from '@/components/layout/build-readout-lines'

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

const NET_INK: Record<NetState, string | undefined> = {
  ok: undefined,
  idle: undefined,
  reconnecting: 'text-warning',
  down: 'text-destructive',
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
        {lines.build}
        {SEP}
        <span data-net-state={net} className={cn(lines.netWarn && NET_INK[net])}>
          {lines.net}
        </span>
        {lines.reconnects && SEP + lines.reconnects}
      </span>
      <span>{lines.stats}</span>
    </>
  )
}

/**
 * Build, daemon connection and frame stats, set like a net-graph: pale yellow
 * monospace with a dark outline, straight on the chrome. Purely visual: it
 * takes no layout space and no pointer events, so a tab under it stays
 * clickable. The frame loop and the store watchers run only while this is
 * shown (Settings → Appearance → Build readout).
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
        'pointer-events-none absolute inset-y-0 z-50 flex select-none flex-col items-start justify-center whitespace-pre font-mono text-[10px] leading-3 tabular-nums [text-shadow:0_0_2px_#000,0_1px_1px_#000,1px_0_1px_#000]',
        className,
      )}
    >
      <Lines info={info} frame={frame} />
    </div>
  )
}
