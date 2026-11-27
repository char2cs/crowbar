import { useSyncExternalStore } from 'react'

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

export function useThemeVersion(): number {
  return useSyncExternalStore(subscribe, getVersion, getServerVersion)
}

export function isDarkMode(): boolean {
  return typeof document !== 'undefined' && document.documentElement.classList.contains('dark')
}
