import {
  cloneGhostTab,
  ghostTransform,
  type GhostRows,
  type GrabOffset,
} from '@/components/layout/drag-ghost'

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
const FALLBACK_TAB_PX = 160
/** The gap an outside subject opens: every tab from the insertion index on makes room. */
function gapOffsets(count: number, to: number, pitch: number): number[] {
  return Array.from({ length: count }, (_, i) => (i >= to ? pitch : 0))
}

/** The insertion index: how many whole tabs lie left of the subject's centre, so a subject sitting on a slot takes it. */
function insertIndex(slots: readonly TabSlot[], centerX: number): number {
  return slots.filter((slot) => slot.left + slot.width <= centerX).length
}

// Pointer travel past this before a slide changes hands, so a boundary does not flicker.
const HYSTERESIS_PX = 6

/**
 * A tab dragged within its own strip, or a subject from outside it (a pane
 * dragged up) previewed as a tab. Owns no React state: the band is derived
 * from the pointer on every move, and the tabs slide by direct transforms so
 * a drag re-renders nothing.
 */
export interface StripDrag {
  inBand: (x: number, y: number) => boolean
  /** The grab point within the ghost the drag draws. */
  grab: GrabOffset
  /** Where the ghost sits for a pointer at (x, y): its first paint and every move. */
  position: (x: number, y: number) => { x: number; y: number }
  /** What the ghost shows: the whole tab, or just the room for a pane's own preview. */
  ghostRows: () => GhostRows
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

const glide = (el: HTMLElement) => {
  el.style.transition = `transform ${SLIDE_MS}ms ease-out`
  el.style.transform = ''
  const done = () => {
    el.style.transition = ''
    el.removeEventListener('transitionend', done)
  }
  el.addEventListener('transitionend', done)
}

export function createStripDrag(
  strip: HTMLElement,
  subject: HTMLElement,
  pressed: GrabOffset,
): StripDrag | null {
  const tabs = Array.from(strip.children).filter((c): c is HTMLElement => c instanceof HTMLElement)
  if (tabs.length === 0) return null
  const index = tabs.findIndex((tab) => tab.contains(subject))
  const external = index < 0
  // An outside subject is the virtual tab after the last one, which keeps every formula below.
  const from = external ? tabs.length : index

  // Horizontal extent is the strip's; vertical is the tabs', as the strip carries clip room above and below them.
  const stripRect = strip.getBoundingClientRect()
  const tabRect = tabs[0].getBoundingClientRect()
  const box = {
    left: stripRect.left,
    right: stripRect.right,
    top: tabRect.top,
    bottom: tabRect.top + tabRect.height,
    height: tabRect.height,
  }
  const scroll0 = strip.scrollLeft
  const slots = tabs.map((tab) => {
    const r = tab.getBoundingClientRect()
    return { left: r.left, width: r.width }
  })
  const shellOf = (tab: HTMLElement) => tab.firstElementChild ?? tab
  // A pinned tab moves among the pinned, an unpinned one (or a pane dragged up) among the rest.
  const pinnedCount = tabs.filter((tab) => tab.hasAttribute('data-pinned')).length
  const pinnedSubject = !external && from < pinnedCount
  const lowest = pinnedSubject ? 0 : pinnedCount
  const highest = pinnedSubject ? pinnedCount - 1 : tabs.length - 1
  // The size of an ordinary tab, which a pane's preview takes; every tab pinned leaves none to copy.
  const model = external ? Math.min(pinnedCount, tabs.length - 1) : from
  const shell = shellOf(tabs[model]).getBoundingClientRect()
  const subjectRect = subject.getBoundingClientRect()
  const margin = box.height
  const modelIsPinned = external && pinnedCount === tabs.length
  const width = modelIsPinned ? FALLBACK_TAB_PX : shell.width
  const pitch = modelIsPinned ? FALLBACK_TAB_PX + 4 : slots[model].width
  // Held where the pointer took it, within the whole tab; an outside subject is held by its centre.
  const grab = external
    ? { dx: width / 2, dy: shell.height / 2 }
    : {
        dx: pressed.dx + subjectRect.left - shell.left,
        dy: pressed.dy + subjectRect.top - shell.top,
      }
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
  // The tab travels over the tabs themselves, not the empty strip beyond the last one. An outside
  // subject lands exactly on the first or last slot, and hangs half off only where the strip is full.
  const leftFor = (x: number) => {
    const now = slotsNow()
    const end = now[highest]
    const start = now[lowest] ?? end
    const first = lowest < now.length ? start.left : start.left + start.width
    const last = end.left + end.width
    const half = width / 2
    const reach = external
      ? {
          ...box,
          left: Math.max(first, box.left - half),
          right: Math.min(last, box.right - half) + width,
        }
      : { ...box, left: Math.max(box.left, first), right: Math.min(box.right, last) }
    return clampTabLeft(x, grab.dx, width, reach)
  }
  const indexAt = (x: number, shift: number) => {
    const left = leftFor(x + shift)
    return external
      ? insertIndex(slotsNow(), left + width / 2)
      : targetIndex(slotsNow(), from, left, left + width)
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
    const offsets = external
      ? gapOffsets(tabs.length, to, pitch)
      : slideOffsets(tabs.length, from, to, pitch)
    tabs.forEach((tab, i) => {
      tab.style.transform = offsets[i] === 0 ? '' : `translateX(${offsets[i]}px)`
    })
  }

  // Pinned to its strip row inside the strip, then eased toward the free pointer across the band,
  // so leaving it carries no jump: the ghost reaches the pointer exactly at the band's edge.
  function ghostPosition(x: number, y: number) {
    const pinned = { x: leftFor(x), y: shell.top }
    const k = Math.min(1, outside(x, y) / margin)
    return {
      x: pinned.x + (x - grab.dx - pinned.x) * k,
      y: pinned.y + (y - grab.dy - pinned.y) * k,
    }
  }

  // The new tab only exists once the drop has rendered: start it at the ghost the moment it does.
  function settleNewTab(x: number, at: { x: number; y: number }) {
    const to = indexFor(x)
    const left = slotsNow()[Math.min(to, tabs.length - 1)]
    const finalLeft = to < tabs.length ? left.left : left.left + left.width
    const known = new Set<Element>(tabs)
    const watch = new MutationObserver(() => {
      const added = Array.from(strip.children).find((c) => !known.has(c))
      if (!(added instanceof HTMLElement)) return
      watch.disconnect()
      added.style.transition = 'none'
      added.style.transform = `translate(${at.x - finalLeft}px, ${at.y - shell.top}px)`
      requestAnimationFrame(() => glide(added))
    })
    watch.observe(strip, { childList: true })
    requestAnimationFrame(() => watch.disconnect())
  }

  return {
    inBand,
    grab,
    position: ghostPosition,
    ghostRows: () => {
      if (external) return { nodes: [], width, height: shell.height, count: 1 }
      return cloneGhostTab(shellOf(tabs[from]) as HTMLElement)
    },
    move(x, y, ghost) {
      const now = inBand(x, y)
      // The source slot is an empty gap while its tab is in the strip, and the
      // usual faded row once the ghost has left it.
      if (!external && now !== hidden) tabs[from].style.visibility = now ? 'hidden' : ''
      hidden = now
      paintSlide(now ? indexFor(x) : from)
      if (!now) current = from
      ghostAt = ghostPosition(x, y)
      if (ghost) ghost.style.transform = ghostTransform(ghostAt.x, ghostAt.y)
    },
    drop(x) {
      const to = indexFor(x)
      if (external) {
        return to < tabs.length
          ? { el: tabs[to], mode: 'before' }
          : { el: tabs[tabs.length - 1], mode: 'after' }
      }
      const beside = dropBeside(tabs.length, from, to)
      return beside && { el: tabs[beside.index], mode: beside.mode }
    },
    settle(x) {
      if (!ghostAt) return
      if (external) return settleNewTab(x, ghostAt)
      const slot = slotsNow()[indexFor(x)]
      const tab = tabs[from]
      tab.style.transition = 'none'
      tab.style.transform = `translate(${ghostAt.x - slot.left}px, ${ghostAt.y - shell.top}px)`
      settling = tab
      requestAnimationFrame(() => glide(tab))
    },
    dispose() {
      for (const tab of tabs) {
        if (tab === settling) continue
        tab.style.transition = ''
        tab.style.transform = ''
      }
      if (!external) tabs[from].style.visibility = ''
    },
  }
}
