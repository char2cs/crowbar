import { useSyncExternalStore } from 'react'

// Reactivity seam: the app flips light/dark by toggling a `dark` class on
// `document.documentElement` (see settings-effects.ts) rather than through
// any store a React tree could subscribe to. A plain React render needs an
// actual subscription to know when to re-run, so every theme-sensitive
// renderer (mermaid, Excalidraw) shares this ONE MutationObserver rather
// than each keeping its own.
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
 *  a re-render; callers read the actual mode via `isDarkMode()`. */
export function useThemeVersion(): number {
  return useSyncExternalStore(subscribe, getVersion, getServerVersion)
}

/** The `.dark` class gates all Tailwind `dark:` variants (settings-effects.ts)
 *  — this is the same signal every theme-sensitive consumer already reads. */
export function isDarkMode(): boolean {
  return typeof document !== 'undefined' && document.documentElement.classList.contains('dark')
}
