import { useSyncExternalStore } from 'react'
import { resolveCssVar } from '@/features/editor/theme/resolve-css-color'

// Mirrors the fallback pattern in use-terminal-theme.ts: resolveCssVar reads
// live off the DOM and can return null (run before theme.css has painted, or
// in an environment with no stylesheet at all — a stray unit test), so every
// token is paired with a light/dark literal rather than letting mermaid fall
// through to `undefined`.
function isDarkMode(): boolean {
  return typeof document !== 'undefined' && document.documentElement.classList.contains('dark')
}

function color(name: string, light: string, dark: string): string {
  return resolveCssVar(name) ?? (isDarkMode() ? dark : light)
}

// `resolveCssVar` resolves *color* custom properties (it pipes the raw value
// through an element's `color` style and reads back `rgb()`); `--font-sans`
// is a font-family stack, not a color, so it needs the same "assign to a
// concrete property, read the computed value back" trick applied to
// `font-family` instead.
function resolveFontFamily(): string {
  if (typeof document === 'undefined') return 'inherit'
  const probe = document.createElement('span')
  probe.style.fontFamily = 'var(--font-sans)'
  document.body.appendChild(probe)
  const resolved = getComputedStyle(probe).fontFamily
  document.body.removeChild(probe)
  return resolved || 'inherit'
}

/**
 * Mermaid `theme: 'base'` variable set, sourced entirely from the app's CSS
 * tokens (never mermaid's own default palette) so a rendered diagram matches
 * whichever theme — light, dark, or a future custom one — is currently
 * painted. Pure and side-effect-free; called fresh before every render so a
 * theme change picks up on the diagram's next debounced render pass.
 */
export function buildMermaidThemeVariables(): Record<string, string> {
  return {
    background: color('--background', '#faf9f5', '#141413'),
    fontFamily: resolveFontFamily(),
    primaryColor: color('--secondary', '#ece9df', '#2a2a28'),
    primaryTextColor: color('--foreground', '#141413', '#f5f5f5'),
    primaryBorderColor: color('--border', '#e5e3da', '#3a3a38'),
    secondaryColor: color('--muted', '#f5f4ee', '#242422'),
    secondaryTextColor: color('--foreground', '#141413', '#f5f5f5'),
    secondaryBorderColor: color('--border', '#e5e3da', '#3a3a38'),
    tertiaryColor: color('--accent', '#f5f4ee', '#242422'),
    tertiaryTextColor: color('--foreground', '#141413', '#f5f5f5'),
    tertiaryBorderColor: color('--border', '#e5e3da', '#3a3a38'),
    lineColor: color('--muted-foreground', '#7c7b74', '#a3a29c'),
    textColor: color('--foreground', '#141413', '#f5f5f5'),
    mainBkg: color('--card', '#ffffff', '#1c1c1a'),
    nodeTextColor: color('--foreground', '#141413', '#f5f5f5'),
    edgeLabelBackground: color('--background', '#faf9f5', '#141413'),
    clusterBkg: color('--muted', '#f5f4ee', '#242422'),
    clusterBorder: color('--border', '#e5e3da', '#3a3a38'),
    defaultLinkColor: color('--muted-foreground', '#7c7b74', '#a3a29c'),
    titleColor: color('--foreground', '#141413', '#f5f5f5'),
    errorBkgColor: color('--destructive', '#ef4444', '#ef4444'),
    errorTextColor: color('--destructive-foreground', '#ffffff', '#ffffff'),
    noteBkgColor: color('--accent', '#f5f4ee', '#242422'),
    noteTextColor: color('--accent-foreground', '#141413', '#f5f5f5'),
    noteBorderColor: color('--border', '#e5e3da', '#3a3a38'),
  }
}

// Reactivity seam: the app flips light/dark by toggling a `dark` class on
// `document.documentElement` (see settings-effects.ts) rather than through
// any store a React tree could subscribe to. Every other CSS-var consumer
// (terminal, Monaco) re-reads imperatively on its own trigger; a Mermaid
// diagram is a plain React render, so it needs an actual subscription to
// know when to re-run `mermaid.render` with fresh colors. One shared
// MutationObserver (not one per diagram) backs every subscriber.
let version = 0
const listeners = new Set<() => void>()
let observer: MutationObserver | null = null

function ensureObserver(): void {
  if (observer || typeof document === 'undefined' || typeof MutationObserver === 'undefined') {
    return
  }
  observer = new MutationObserver(() => {
    version++
    listeners.forEach((listener) => listener())
  })
  observer.observe(document.documentElement, { attributes: true, attributeFilter: ['class'] })
}

function subscribe(listener: () => void): () => void {
  ensureObserver()
  listeners.add(listener)
  return () => listeners.delete(listener)
}

function getVersion(): number {
  return version
}

function getServerVersion(): number {
  return 0
}

/** Bumps whenever the app's light/dark class flips — read it purely to force
 *  a re-render; the actual colors come from `buildMermaidThemeVariables()`. */
export function useMermaidThemeVersion(): number {
  return useSyncExternalStore(subscribe, getVersion, getServerVersion)
}
