import { useSyncExternalStore } from 'react'
import { getBuildInfo } from '@/lib/build-info'
import type { BuildChannel, BuildInfo } from '@/lib/build-info'
import { useSettingsStore } from '@/features/settings/store'
import { useConsoleStore } from '@/features/console/stores/console-store'
import { cn } from '@/utils/cn'
import skyUrl from '@/assets/band-sky.png'
import nightUrl from '@/assets/band-night.png'
import grainUrl from '@/assets/band-grain.png'
import grain2xUrl from '@/assets/band-grain@2x.png'

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
  useSyncExternalStore(
    subscribeDark,
    () => darkVersion,
    () => 0,
  )
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
  nightly: { dark: '#7DD3FC', light: '#FFFFFF' },
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

// Halftone sky (light) / star field (dark), screened offline; the dark one is
// white dots on transparent so the sidebar ground shows through.
const GRAIN =
  "url(\"data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='200' height='200'%3E%3Cfilter id='g'%3E%3CfeTurbulence type='fractalNoise' baseFrequency='.85' numOctaves='2' stitchTiles='stitch'/%3E%3CfeColorMatrix values='0 0 0 0 .5 0 0 0 0 .5 0 0 0 0 .5 0 0 0 1 0'/%3E%3C/filter%3E%3Crect width='100%25' height='100%25' filter='url(%23g)'/%3E%3C/svg%3E\")"

// While a pane is being dragged, index.css swaps GRAIN for this rasterised copy: WebKit
// re-renders the SVG filter on every resize frame. Fetched now so the swap never loads.
if (typeof Image !== 'undefined') {
  new Image().src = window.devicePixelRatio >= 1.5 ? grain2xUrl : grainUrl
}

const PHOTO = {
  light: {
    src: skyUrl,
    base: '#24395f',
    scrim: '#24395f',
    crop: { right: 0, top: -104 },
  },
  dark: {
    src: nightUrl,
    base: 'transparent',
    scrim: '#212121',
    crop: { left: 0, top: -60 },
  },
} as const

function BandPhoto({ isDark }: { isDark: boolean }) {
  const { src, scrim, crop } = PHOTO[isDark ? 'dark' : 'light']
  return (
    <>
      <img
        src={src}
        alt=""
        loading="lazy"
        decoding="async"
        draggable={false}
        data-band-photo={isDark ? 'dark' : 'light'}
        className="absolute max-w-none select-none"
        style={{ width: 640, height: 272, ...crop }}
      />
      <div
        data-band-grain=""
        className="absolute inset-0 opacity-10 mix-blend-multiply"
        style={{ background: GRAIN }}
      />
      {/* Flat deep ground under the label so it reads over the dots. */}
      <div
        className="absolute inset-0"
        style={{
          background: `linear-gradient(to right, transparent 25%, ${scrim} 50%, ${scrim} 68%, transparent 92%)`,
        }}
      />
    </>
  )
}

/**
 * Full-bleed decorative background for the build channel — absolutely
 * positioned behind the traffic-light reserve, the badge text, and the
 * back/forward/panel-toggle cluster, none of which it changes.
 *
 * `align` is the SAME fact `SidebarBuildBadgeLabel` already takes — which
 * edge of the header the badge TEXT sits against, i.e. the true outer
 * (window-border) edge of this bar. The decorative art (stars/clouds/dev
 * traces) is authored right-biased in its own 256-wide viewBox, and the
 * fade below is authored fading OUT to the right — both correct only for
 * `align="end"` (sidebar on the right, text/window-edge on this bar's own
 * right). For `align="start"` (sidebar on the left, text/window-edge on
 * this bar's own LEFT — the traffic-light side), both are mirrored via a
 * single `scaleX(-1)`: reported live as the graphics landing BEHIND the
 * back/forward/panel-toggle cluster instead of hugging the window's real
 * edge, because this band never accounted for which side that cluster
 * actually ended up on.
 */
export function SidebarBuildBadgeBand({
  className,
  align = 'end',
}: {
  className?: string
  align?: 'start' | 'end'
}) {
  const info = useResolvedBuildInfo()
  const isDark = useIsDarkMode()
  if (!info || info.channel === 'release') return null

  // linear-gradient(to X, ...) is a PHYSICAL keyword (left/right), not a
  // logical one — 'start'/'end' would silently fail to parse. `align="end"`
  // is this app's own left-to-right convention for "the text/window-edge
  // side is on the right" (see the caller, sidebar-project-header.tsx).
  const fadeToward = align === 'end' ? 'right' : 'left'

  return (
    <div
      className={className}
      aria-hidden="true"
      data-slot="build-badge-band"
      data-channel={info.channel}
    >
      <div
        className="absolute inset-0"
        style={{
          background:
            info.channel === 'nightly'
              ? PHOTO[isDark ? 'dark' : 'light'].base
              : bandBackground(info.channel, isDark),
          // Fades OUT toward the button cluster's side, staying fully
          // opaque at the badge-text/window-edge side — this is the band's
          // own gradient/pattern fill, not just the decorative art above
          // it. `linear-gradient(to X, ...)` anchors the LAST color (black,
          // opaque) at edge X and the FIRST (transparent) at the opposite
          // edge — X must be the text side (`fadeToward`), not its opposite
          // (caught live: had these swapped, which faded out the text side
          // and left the button side solid instead).
          maskImage: `linear-gradient(to ${fadeToward}, transparent 40%, black 70%)`,
          WebkitMaskImage: `linear-gradient(to ${fadeToward}, transparent 40%, black 70%)`,
        }}
      >
        {info.channel === 'nightly' && (
          <div
            className="pointer-events-none absolute inset-0 overflow-hidden"
            style={align === 'start' ? { transform: 'scaleX(-1)' } : undefined}
          >
            <BandPhoto isDark={isDark} />
          </div>
        )}
      </div>
      {info.channel === 'dev' && (
        <div
          className="pointer-events-none absolute inset-0 overflow-hidden"
          style={align === 'start' ? { transform: 'scaleX(-1)' } : undefined}
        >
          <DevTraces />
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
  const consoleOpen = useConsoleStore((s) => s.open)
  const toggleConsole = useConsoleStore((s) => s.toggle)
  if (!info) return null

  const title = info.channel === 'release' ? null : info.channel
  // Nightly in light mode is a "daily" pun — display text only, the channel
  // (and its color) underneath stays 'nightly'.
  const displayTitle = title === 'nightly' && !isDark ? 'daily' : title
  const subtitle = info.version ?? formatTimestamp(info.timestamp)
  const titleColor = title ? TITLE_COLOR[title][isDark ? 'dark' : 'light'] : undefined
  const onSky = title === 'nightly' && !isDark

  return (
    <button
      type="button"
      aria-label="Toggle console"
      aria-expanded={consoleOpen}
      aria-controls="console-panel"
      onClick={toggleConsole}
      className={cn(
        'flex cursor-pointer flex-col justify-center gap-px font-mono leading-tight outline-none focus-visible:ring-2 focus-visible:ring-ring',
        align === 'end' ? 'items-end text-right' : 'items-start text-left',
      )}
    >
      {displayTitle && (
        <span className="text-xs font-bold tracking-tight" style={{ color: titleColor }}>
          {displayTitle}
        </span>
      )}
      {subtitle && (
        <span
          className={cn(
            'text-[9.5px] font-medium',
            onSky ? 'text-white/85' : 'text-muted-foreground',
          )}
        >
          {subtitle}
        </span>
      )}
    </button>
  )
}
