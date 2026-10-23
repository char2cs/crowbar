import { getBuildInfo } from '@/lib/build-info'
import type { BuildChannel, BuildInfo } from '@/lib/build-info'
import { useSettingsStore } from '@/features/settings/store'
import { cn } from '@/utils/cn'
import { useFrameStats } from '@/lib/frame-stats'

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

function formatTimestamp(iso?: string): string {
  if (!iso) return ''
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`
}

const READOUT_INK = '#E0E0D0'

function Stat({ name, value }: { name: string; value: string }) {
  return (
    <span>
      {name}: {value}
    </span>
  )
}

/**
 * `channel - time` (or version) over the frame stats, set like a net-graph:
 * pale yellow monospace with a dark outline, straight on the chrome. Purely
 * visual: it takes no layout space and no pointer events, so a tab under it
 * stays clickable. The frame stats need a per-frame loop, which runs only while
 * this is shown (Settings → Appearance → Build readout).
 */
export function BuildReadout({ className }: { className?: string }) {
  const info = useResolvedBuildInfo()
  const visible = useSettingsStore((s) => s.settings.showBuildReadout)
  const { fps, maxDt, drops } = useFrameStats(visible)
  if (!visible || !info) return null

  const build = [
    info.channel === 'release' ? '' : info.channel,
    info.version ?? formatTimestamp(info.timestamp),
  ]
    .filter(Boolean)
    .join(' - ')

  return (
    <div
      aria-hidden="true"
      style={{ color: READOUT_INK }}
      className={cn(
        'pointer-events-none absolute inset-y-0 z-50 flex select-none flex-col items-start justify-center whitespace-nowrap font-mono text-[10px] leading-3 tabular-nums [text-shadow:0_0_2px_#000,0_1px_1px_#000,1px_0_1px_#000]',
        className,
      )}
    >
      <span>{build}</span>
      <span className="flex gap-3">
        <Stat name="fps" value={fps > 0 ? String(fps) : '--'} />
        <Stat name="max" value={maxDt > 0 ? `${maxDt} ms` : '--'} />
        <Stat name="drops" value={String(drops)} />
      </span>
    </div>
  )
}
