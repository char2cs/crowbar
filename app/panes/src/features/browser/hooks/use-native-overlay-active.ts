import { useSyncExternalStore } from 'react'

const NON_BLOCKING = '[data-slot="tooltip-content"], [data-slot*="toast"]'
const IGNORED_TAGS = new Set(['SCRIPT', 'STYLE', 'LINK', 'NOSCRIPT', 'INPUT', 'TEXTAREA', 'SELECT'])
const INSET_CLIPS_ALL = /^inset\(100%|^inset\((?:100|[5-9]\d(?:\.\d+)?)%\)/
const RECT =
  /^rect\(\s*(-?[\d.]+)(?:px)?,?\s+(-?[\d.]+)(?:px)?,?\s+(-?[\d.]+)(?:px)?,?\s+(-?[\d.]+)(?:px)?\s*\)/

function clipsEverything(clip: string): boolean {
  const m = RECT.exec(clip)
  if (!m) return false
  const [top, right, bottom, left] = m.slice(1).map(Number)
  return top >= bottom || left >= right
}

function cannotPaint(el: Element): boolean {
  if (IGNORED_TAGS.has(el.tagName) || el.hasAttribute('hidden')) return true
  const style = getComputedStyle(el)
  return (
    style.display === 'none' ||
    style.visibility === 'hidden' ||
    parseFloat(style.opacity) === 0 ||
    INSET_CLIPS_ALL.test(style.clipPath) ||
    clipsEverything(style.clip)
  )
}

function isBlocking(child: Element): boolean {
  if (child.id === 'root' || cannotPaint(child)) return false
  const slots = [child, ...child.querySelectorAll('[data-slot]')].filter((el) =>
    el.hasAttribute('data-slot'),
  )
  return slots.length === 0 || slots.some((el) => !el.closest(NON_BLOCKING))
}

function snapshot(): boolean {
  return Array.from(document.body.children).some(isBlocking)
}

const listeners = new Set<() => void>()
let observer: MutationObserver | null = null

function subscribe(notify: () => void): () => void {
  listeners.add(notify)
  if (!observer) {
    observer = new MutationObserver(() => listeners.forEach((l) => l()))
    observer.observe(document.body, { childList: true })
  }
  return () => {
    listeners.delete(notify)
    if (listeners.size > 0) return
    observer?.disconnect()
    observer = null
  }
}

export function useNativeOverlayActive(): boolean {
  return useSyncExternalStore(subscribe, snapshot)
}
