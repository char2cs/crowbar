import { useSyncExternalStore } from 'react'

// Light/dark flips by a `dark` class on <html> (settings-effects.ts), not a store React can
// subscribe to, so theme-sensitive renderers (mermaid, Excalidraw) share this one
// MutationObserver as their subscription.
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
