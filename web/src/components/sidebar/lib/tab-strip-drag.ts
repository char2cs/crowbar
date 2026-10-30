import { ghostTransform, type GrabOffset } from '@/components/layout/drag-ghost'

/** A viewport box; the strip's own, and the margin-grown band around it. */
interface StripBox {
  left: number
  right: number
  top: number
  bottom: number
}

/** Where a tab sat before the drag moved anything. */
interface TabSlot {
  left: number
  width: number
}

/** The dragged tab's left edge: under the grab point, never past either end of the strip. */
function clampTabLeft(pointerX: number, grabDx: number, width: number, box: StripBox) {
  return Math.max(box.left, Math.min(pointerX - grabDx, box.right - width))
}

/**
 * The index the dragged tab takes: it passes a tab once its leading edge
 * crosses that tab's midpoint, so the clamped ends reach the first and last slot.
 */
function targetIndex(slots: readonly TabSlot[], from: number, left: number, right: number): number {
  let index = from
  slots.forEach((slot, i) => {
    const mid = slot.left + slot.width / 2
    if (i < from && left < mid) index -= 1
    if (i > from && right > mid) index += 1
  })
  return index
}

/** How far each tab slides so the gap lands at `to`; the dragged tab's own slot stays at 0. */
function slideOffsets(count: number, from: number, to: number, pitch: number): number[] {
  return Array.from({ length: count }, (_, i) => {
    if (i === from) return 0
    if (i > from && i <= to) return -pitch
    if (i < from && i >= to) return pitch
    return 0
  })
}

/** The tab a release at `to` lands beside, and on which side; null when nothing moves. */
function dropBeside(
  count: number,
  from: number,
  to: number,
): { index: number; mode: 'before' | 'after' } | null {
  if (to === from) return null
  const others = Array.from({ length: count }, (_, i) => i).filter((i) => i !== from)
  return to < others.length
    ? { index: others[to], mode: 'before' }
    : { index: others[others.length - 1], mode: 'after' }
}

const SLIDE_MS = 150
// Pointer travel past this before a slide changes hands, so a boundary does not flicker.
const HYSTERESIS_PX = 6

/**
 * A tab dragged within its own strip. Owns no React state: the band is
 * derived from the pointer on every move, and the tabs slide by direct
 * transforms so a drag re-renders nothing.
 */
export interface StripDrag {
  inBand: (x: number, y: number) => boolean
  /** Re-aim the slide and the ghost from the pointer (and the strip's scroll). */
  move: (x: number, y: number, ghost: HTMLElement | null) => void
  /** Where a release lands: the neighbouring tab's element, or null if nothing moves. */
  drop: (x: number) => { el: HTMLElement; mode: 'before' | 'after' } | null
  /** Start the dropped tab where the ghost was, so it glides into its slot instead of teleporting. */
  settle: (x: number) => void
  /** Give every tab back its own place. */
  dispose: () => void
}

const distanceOutside = (v: number, lo: number, hi: number) =>
  v < lo ? lo - v : v > hi ? v - hi : 0

export function createStripDrag(
  strip: HTMLElement,
  grabbed: HTMLElement,
  grab: GrabOffset,
): StripDrag | null {
  const tabs = Array.from(strip.children).filter((c): c is HTMLElement => c instanceof HTMLElement)
  const from = tabs.findIndex((tab) => tab.contains(grabbed))
  if (from < 0) return null

  const box = strip.getBoundingClientRect()
  const grabbedRect = grabbed.getBoundingClientRect()
  const scroll0 = strip.scrollLeft
  const slots = tabs.map((tab) => {
    const r = tab.getBoundingClientRect()
    return { left: r.left, width: r.width }
  })
  const margin = box.height
  const width = grabbedRect.width
  const pitch = slots[from].width
  let current = from
  let shown = -1
  let hidden = false
  let ghostAt: { x: number; y: number } | null = null
  let settling: HTMLElement | null = null

  for (const tab of tabs) tab.style.transition = `transform ${SLIDE_MS}ms ease-out`

  const slotsNow = () => slots.map((s) => ({ ...s, left: s.left - (strip.scrollLeft - scroll0) }))
  const outside = (x: number, y: number) =>
    Math.max(distanceOutside(x, box.left, box.right), distanceOutside(y, box.top, box.bottom))
  const inBand = (x: number, y: number) => outside(x, y) <= margin
  // The tab travels over the tabs themselves, not the empty strip beyond the last one.
  const leftFor = (x: number) => {
    const now = slotsNow()
    const last = now[now.length - 1]
    const reach = {
      ...box,
      left: Math.max(box.left, now[0].left),
      right: Math.min(box.right, last.left + last.width),
    }
    return clampTabLeft(x, grab.dx, width, reach)
  }
  const indexAt = (x: number, shift: number) => {
    const left = leftFor(x) + shift
    return targetIndex(slotsNow(), from, left, left + width)
  }
  // Moves only once the pointer is past the boundary by the hysteresis, in either direction.
  function indexFor(x: number) {
    const lo = indexAt(x, -HYSTERESIS_PX)
    const hi = indexAt(x, HYSTERESIS_PX)
    current = Math.min(Math.max(current, lo), hi)
    return current
  }

  function paintSlide(to: number) {
    if (to === shown) return
    shown = to
    const offsets = slideOffsets(tabs.length, from, to, pitch)
    tabs.forEach((tab, i) => {
      tab.style.transform = offsets[i] === 0 ? '' : `translateX(${offsets[i]}px)`
    })
  }

  // Pinned to its strip row inside the strip, then eased toward the free pointer across the band,
  // so leaving it carries no jump: the ghost reaches the pointer exactly at the band's edge.
  function ghostPosition(x: number, y: number) {
    const pinned = { x: leftFor(x), y: grabbedRect.top }
    const k = Math.min(1, outside(x, y) / margin)
    return {
      x: pinned.x + (x - grab.dx - pinned.x) * k,
      y: pinned.y + (y - grab.dy - pinned.y) * k,
    }
  }

  return {
    inBand,
    move(x, y, ghost) {
      const now = inBand(x, y)
      // The source slot is an empty gap while its tab is in the strip, and the
      // usual faded row once the ghost has left it.
      if (now !== hidden) tabs[from].style.visibility = now ? 'hidden' : ''
      hidden = now
      paintSlide(now ? indexFor(x) : from)
      if (!now) current = from
      ghostAt = ghostPosition(x, y)
      if (ghost) ghost.style.transform = ghostTransform(ghostAt.x, ghostAt.y)
    },
    drop(x) {
      const beside = dropBeside(tabs.length, from, indexFor(x))
      return beside && { el: tabs[beside.index], mode: beside.mode }
    },
    settle(x) {
      if (!ghostAt) return
      const slot = slotsNow()[indexFor(x)]
      const tab = tabs[from]
      tab.style.transition = 'none'
      tab.style.transform = `translate(${ghostAt.x - slot.left}px, ${ghostAt.y - grabbedRect.top}px)`
      settling = tab
      requestAnimationFrame(() => {
        tab.style.transition = `transform ${SLIDE_MS}ms ease-out`
        tab.style.transform = ''
        const done = () => {
          tab.style.transition = ''
          tab.removeEventListener('transitionend', done)
        }
        tab.addEventListener('transitionend', done)
      })
    },
    dispose() {
      for (const tab of tabs) {
        if (tab === settling) continue
        tab.style.transition = ''
        tab.style.transform = ''
      }
      tabs[from].style.visibility = ''
    },
  }
}
