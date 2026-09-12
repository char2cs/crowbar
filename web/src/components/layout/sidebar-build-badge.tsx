import { useSyncExternalStore } from 'react'
import { getBuildInfo } from '@/lib/build-info'
import type { BuildChannel, BuildInfo } from '@/lib/build-info'
import { useSettingsStore } from '@/features/settings/store'
import { cn } from '@/utils/cn'

// Same MutationObserver-on-`.dark`-class pattern as
// features/editor/markdown/plate/mermaid-theme.ts's useMermaidThemeVersion —
// kept as its own tiny copy here rather than a shared import so this
// self-contained sidebar-chrome file has no cross-feature dependency.
let darkVersion = 0
const darkListeners = new Set<() => void>()
let darkObserver: MutationObserver | null = null

function ensureDarkObserver(): void {
  if (darkObserver || typeof document === 'undefined' || typeof MutationObserver === 'undefined') {
    return
  }
  darkObserver = new MutationObserver(() => {
    darkVersion++
    darkListeners.forEach((listener) => listener())
  })
  darkObserver.observe(document.documentElement, { attributes: true, attributeFilter: ['class'] })
}

function subscribeDark(listener: () => void): () => void {
  ensureDarkObserver()
  darkListeners.add(listener)
  return () => darkListeners.delete(listener)
}

function getDarkSnapshot(): boolean {
  return typeof document !== 'undefined' && document.documentElement.classList.contains('dark')
}

function useIsDarkMode(): boolean {
  useSyncExternalStore(subscribeDark, () => darkVersion, () => 0)
  return getDarkSnapshot()
}

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

const TITLE_COLOR: Record<Exclude<BuildChannel, 'release'>, { dark: string; light: string }> = {
  dev: { dark: '#8fb0f2', light: '#3E5CB8' },
  nightly: { dark: '#7DD3FC', light: '#0369A1' },
  beta: { dark: '#FBBF24', light: '#B45309' },
}

function bandBackground(channel: BuildChannel, isDark: boolean): string | undefined {
  if (channel === 'dev') {
    return isDark
      ? 'repeating-linear-gradient(0deg, rgba(148,177,235,0.09) 0 1px, transparent 1px 22px), repeating-linear-gradient(90deg, rgba(148,177,235,0.09) 0 1px, transparent 1px 22px), radial-gradient(circle at 8% 24%, rgba(124,156,230,0.2) 1.6px, transparent 2px), radial-gradient(circle at 24% 68%, rgba(124,156,230,0.15) 1.6px, transparent 2px), linear-gradient(135deg, #0d1836 0%, #14224b 60%, #182a5c 100%)'
      : 'repeating-linear-gradient(0deg, rgba(62,92,184,0.09) 0 1px, transparent 1px 22px), repeating-linear-gradient(90deg, rgba(62,92,184,0.09) 0 1px, transparent 1px 22px), radial-gradient(circle at 8% 24%, rgba(62,92,184,0.18) 1.6px, transparent 2px), radial-gradient(circle at 24% 68%, rgba(62,92,184,0.13) 1.6px, transparent 2px), linear-gradient(135deg, #e8effd 0%, #dbe6fb 60%, #cfdefa 100%)'
  }
  if (channel === 'nightly') {
    return isDark
      ? 'radial-gradient(circle at 88% 6%, rgba(120,140,220,0.28) 0%, transparent 55%), linear-gradient(100deg, #0c0f24 0%, #161c42 55%, #212a5c 100%)'
      : 'linear-gradient(100deg, #eaf2ff 0%, #d7e8ff 55%, #c3ddff 100%)'
  }
  if (channel === 'beta') {
    return isDark
      ? 'repeating-linear-gradient(135deg, rgba(251,191,36,0.12) 0 7px, transparent 7px 16px), linear-gradient(120deg, #2c2013 0%, #3a2a15 60%, #4a3419 100%)'
      : 'repeating-linear-gradient(135deg, rgba(180,83,9,0.08) 0 7px, transparent 7px 16px), linear-gradient(120deg, #fdf6e8 0%, #fbecc9 60%, #f8e2ac 100%)'
  }
  return undefined
}

function DevTraces() {
  return (
    <svg width="256" height="44" viewBox="0 0 256 44" className="absolute inset-0 opacity-55">
      <g stroke="rgba(180,200,245,0.55)" strokeWidth={1} fill="none">
        <path d="M148 36 L148 24 L172 24 L172 12 L208 12" />
        <path d="M196 36 L222 36 L222 18 L244 18" />
      </g>
      <g fill="rgba(180,200,245,0.55)">
        <circle cx={148} cy={36} r={1.6} />
        <circle cx={172} cy={24} r={1.6} />
        <circle cx={208} cy={12} r={1.6} />
        <circle cx={222} cy={18} r={1.6} />
        <circle cx={244} cy={18} r={1.6} />
      </g>
    </svg>
  )
}

function NightSky() {
  return (
    <svg width="256" height="44" viewBox="0 0 256 44" className="absolute inset-0">
      <circle cx={230} cy={10} r={1.1} fill="#ffffff" opacity={0.85} />
      <circle cx={206} cy={28} r={0.9} fill="#ffffff" opacity={0.7} />
      <circle cx={188} cy={9} r={0.7} fill="#ffffff" opacity={0.6} />
      <circle cx={150} cy={32} r={0.8} fill="#ffffff" opacity={0.65} />
      <circle cx={122} cy={8} r={0.9} fill="#ffffff" opacity={0.7} />
      <circle cx={96} cy={27} r={0.7} fill="#ffffff" opacity={0.55} />
      <circle cx={241} cy={33} r={0.7} fill="#ffffff" opacity={0.55} />
      <path
        d="M173 6 L174 10.2 L178.2 11.2 L174 12.2 L173 16.4 L172 12.2 L167.8 11.2 L172 10.2 Z"
        fill="#ffffff"
        opacity={0.9}
      />
    </svg>
  )
}

function DaySky() {
  return (
    <svg width="256" height="44" viewBox="0 0 256 44" className="absolute inset-0">
      <g fill="#ffffff" opacity={0.75}>
        <ellipse cx={228} cy={35} rx={13} ry={6} />
        <circle cx={220} cy={31} r={6.5} />
        <circle cx={230} cy={28} r={7.5} />
        <circle cx={239} cy={32} r={6} />
      </g>
      <g fill="#ffffff" opacity={0.5}>
        <ellipse cx={196} cy={30} rx={9} ry={4.5} />
        <circle cx={190} cy={27.5} r={4.5} />
        <circle cx={198} cy={25.5} r={5.5} />
        <circle cx={204} cy={28.5} r={4} />
      </g>
    </svg>
  )
}

/**
 * Full-bleed decorative background for the build channel — absolutely
 * positioned behind the traffic-light reserve, the badge text, and the
 * back/forward/panel-toggle cluster, none of which it changes.
 */
export function SidebarBuildBadgeBand({ className }: { className?: string }) {
  const info = useResolvedBuildInfo()
  const isDark = useIsDarkMode()
  if (!info || info.channel === 'release') return null

  return (
    <div
      className={className}
      style={{ background: bandBackground(info.channel, isDark) }}
      aria-hidden="true"
    >
      {info.channel === 'dev' && (
        <div className="pointer-events-none absolute inset-0 overflow-hidden">
          <DevTraces />
        </div>
      )}
      {info.channel === 'nightly' && (
        <div className="pointer-events-none absolute inset-0 overflow-hidden">
          {isDark ? <NightSky /> : <DaySky />}
        </div>
      )}
    </div>
  )
}

/**
 * Title (channel name) + subtitle (version, or timestamp when there is no
 * version) in the sidebar-header dead space. `align` follows which edge of
 * the header this block sits against — the caller knows that from sidebar
 * position, this component doesn't — so the shorter line hugs the same edge
 * as the longer one instead of always flush-left.
 */
export function SidebarBuildBadgeLabel({ align = 'start' }: { align?: 'start' | 'end' }) {
  const info = useResolvedBuildInfo()
  const isDark = useIsDarkMode()
  if (!info) return null

  const title = info.channel === 'release' ? null : info.channel
  // Nightly in light mode is a "daily" pun — display text only, the channel
  // (and its color) underneath stays 'nightly'.
  const displayTitle = title === 'nightly' && !isDark ? 'daily' : title
  const subtitle = info.version ?? formatTimestamp(info.timestamp)
  const titleColor = title ? TITLE_COLOR[title][isDark ? 'dark' : 'light'] : undefined

  return (
    <div
      className={cn(
        'flex flex-col justify-center gap-px font-mono leading-tight',
        align === 'end' ? 'items-end text-right' : 'items-start text-left',
      )}
    >
      {displayTitle && (
        <span className="text-xs font-bold tracking-tight" style={{ color: titleColor }}>
          {displayTitle}
        </span>
      )}
      {subtitle && (
        <span className="text-[9.5px] font-medium text-muted-foreground">{subtitle}</span>
      )}
    </div>
  )
}
