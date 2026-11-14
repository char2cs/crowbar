import { useSyncExternalStore } from 'react'

// The webview paints above all DOM, so a body-level popup must hide it. Default
// is blocking; only wrappers made solely of tooltip/toast slots are exempt.
const NON_BLOCKING = '[data-slot="tooltip-content"], [data-slot*="toast"]'
const IGNORED_TAGS = new Set(['SCRIPT', 'STYLE', 'LINK', 'NOSCRIPT', 'INPUT', 'TEXTAREA', 'SELECT'])
const INSET_CLIPS_ALL = /^inset\(100%|^inset\((?:100|[5-9]\d(?:\.\d+)?)%\)/
const RECT =
  /^rect\(\s*(-?[\d.]+)(?:px)?,?\s+(-?[\d.]+)(?:px)?,?\s+(-?[\d.]+)(?:px)?,?\s+(-?[\d.]+)(?:px)?\s*\)/

// A clip rect with no area (top >= bottom or left >= right) paints nothing.
function clipsEverything(clip: string): boolean {
  const m = RECT.exec(clip)
  if (!m) return false
  const [top, right, bottom, left] = m.slice(1).map(Number)
  return top >= bottom || left >= right
}

// Persistent helpers libraries park in the body (Slate's shadow input, dnd-kit's
// live regions) are made unpaintable on purpose; that, not their tag, exempts them.
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
  // Observes direct children only; every callback re-reads the live DOM.
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

/** True while a popup, dialog or drag preview sits in the body above the app. */
export function useNativeOverlayActive(): boolean {
  return useSyncExternalStore(subscribe, snapshot)
}
