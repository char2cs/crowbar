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

/** The band is the strip grown by `margin` on every side, like Chrome's detach threshold. */
function inStripBand(x: number, y: number, box: StripBox, margin: number): boolean {
  return (
    x >= box.left - margin &&
    x <= box.right + margin &&
    y >= box.top - margin &&
    y <= box.bottom + margin
  )
}

/** The dragged tab's left edge: under the grab point, never past either end of the strip. */
function clampTabLeft(pointerX: number, grabDx: number, width: number, box: StripBox) {
  return Math.max(box.left, Math.min(pointerX - grabDx, box.right - width))
}

/** The index the dragged tab takes: how many of the other tabs sit left of its centre (a tie counts: the clamped end must reach the last slot). */
function targetIndex(slots: readonly TabSlot[], from: number, centerX: number): number {
  let index = 0
  slots.forEach((slot, i) => {
    if (i !== from && slot.left + slot.width / 2 <= centerX) index += 1
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
  /** Give every tab back its own place. */
  dispose: () => void
}

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
  let wasInBand = false

  for (const tab of tabs) tab.style.transition = `transform ${SLIDE_MS}ms ease-out`

  const slotsNow = () => slots.map((s) => ({ ...s, left: s.left - (strip.scrollLeft - scroll0) }))
  const inBand = (x: number, y: number) => inStripBand(x, y, box, margin)
  const leftFor = (x: number) => clampTabLeft(x, grab.dx, width, box)
  const indexFor = (x: number) => targetIndex(slotsNow(), from, leftFor(x) + width / 2)

  function paintSlide(to: number) {
    const offsets = slideOffsets(tabs.length, from, to, pitch)
    tabs.forEach((tab, i) => {
      tab.style.transform = offsets[i] === 0 ? '' : `translateX(${offsets[i]}px)`
    })
  }

  // A flip between lifted-in-strip and floating glides instead of snapping.
  function glideGhost(ghost: HTMLElement) {
    ghost.style.transition = `transform ${SLIDE_MS}ms ease-out`
    const settle = () => {
      ghost.style.transition = ''
      ghost.removeEventListener('transitionend', settle)
      ghost.removeEventListener('transitioncancel', settle)
    }
    ghost.addEventListener('transitionend', settle)
    ghost.addEventListener('transitioncancel', settle)
  }

  return {
    inBand,
    move(x, y, ghost) {
      const now = inBand(x, y)
      const flipped = now !== wasInBand
      wasInBand = now
      // The source slot is an empty gap while its tab is in the strip, and the
      // usual faded row once the ghost has left it.
      tabs[from].style.visibility = now ? 'hidden' : ''
      paintSlide(now ? indexFor(x) : from)
      if (!ghost) return
      if (flipped) glideGhost(ghost)
      ghost.style.transform = now
        ? ghostTransform(leftFor(x), grabbedRect.top)
        : ghostTransform(x - grab.dx, y - grab.dy)
    },
    drop(x) {
      const beside = dropBeside(tabs.length, from, indexFor(x))
      return beside && { el: tabs[beside.index], mode: beside.mode }
    },
    dispose() {
      for (const tab of tabs) {
        tab.style.transition = ''
        tab.style.transform = ''
      }
      tabs[from].style.visibility = ''
    },
  }
}
