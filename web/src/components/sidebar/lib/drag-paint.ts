import { dropLineBox, dropLineBoxX } from '@/components/layout/drop-indicator'
import { setDropHover } from '@/features/panes/stores/drag-store'
import type { DropMode, RowRect } from '@/components/tree-dnd/drop-core'
import type { PaneDropZone } from '@/features/panes/utils/pane-drop-zones'

interface PaneZoneHit {
  paneId: string
  zone: PaneDropZone | 'center'
}

/**
 * Painted onto the SAME element as {@link PANE_DROP_ATTR} for as long as this
 * pane is the one a release would land in — spec §8.2's "the entry about to
 * take a drop wears the same ring a pane wears" is a promise about a PANE
 * too, and this is the attribute a pane's own CSS reads to draw it (see
 * `paintPaneHit` below). A bare presence flag, like the old veil's
 * `data-armed` — there is only one question here ("is this the one"), not a
 * value to carry.
 */
const PANE_HIT_ATTR = 'data-pane-hit'

export interface DropLine {
  rect: RowRect
  axis: 'x' | 'y'
  mode: DropMode
}

export function paintDropLine(el: HTMLDivElement | null, line: DropLine | null): void {
  if (!el) return
  if (!line || line.mode === 'into') {
    el.style.display = 'none'
    return
  }
  const box =
    line.axis === 'x' ? dropLineBoxX(line.rect, line.mode) : dropLineBox(line.rect, line.mode)
  el.style.display = 'block'
  if (el.dataset.dropIndicator !== line.mode) el.dataset.dropIndicator = line.mode
  const height = 'height' in box ? `${box.height}px` : ''
  if (el.style.height !== height) el.style.height = height
  const left = `${box.left}px`
  const top = `${box.top}px`
  const width = `${box.width}px`
  if (el.style.left !== left) el.style.left = left
  if (el.style.top !== top) el.style.top = top
  if (el.style.width !== width) el.style.width = width
}

export function setDropLineTracking(el: HTMLDivElement | null, tracking: boolean): void {
  if (!el) return
  el.style.transition = tracking ? 'none' : ''
}

export function samePaneHit(a: PaneZoneHit | null, b: PaneZoneHit | null): boolean {
  if (a === null || b === null) return a === b
  return a.paneId === b.paneId && a.zone === b.zone
}

/**
 * §8.2: "the entry about to take a drop wears the same ring a pane wears" —
 * so a pane being hovered during a sidebar-row drag needs the OTHER half of
 * that same answer, a neutral indicator marking which pane a release would
 * land in. Painted straight onto the DOM, like the hairline above and the
 * ghost — a pane's own element lives in a completely different part of the
 * tree (inside `WorkspaceHost`, not the sidebar), so there is no ref to hand
 * this a mounted node the way `attachDropLine` gets one.
 *
 * Takes the RESOLVED elements, never re-derives one by attribute (Fix round
 * 2): `PANE_DROP_ATTR`'s value is not unique across the document — every
 * retained workspace's hidden pane tree carries the same
 * `ROOT_PANE_ID`/`BOTTOM_PANE_ID` strings — so a `document.querySelector`
 * lookup here could paint a different, off-screen workspace's node instead
 * of the one the pointer is actually over, depending on DOM order. `el` is
 * whatever `elementsFromPoint` itself resolved (see `ResolvedPaneHit`), which
 * cannot make that mistake — it only ever names what is really painted
 * there.
 *
 * Gated on the ELEMENT changing, not the paneId string — for the same
 * reason: two elements can share a paneId, and only element identity says
 * "this is genuinely a different target" (a center→edge move on the SAME
 * pane repaints the drop line but not this).
 */
export function paintPaneHit(prev: { el: Element } | null, next: { el: Element } | null): void {
  if ((prev?.el ?? null) === (next?.el ?? null)) return
  prev?.el.removeAttribute(PANE_HIT_ATTR)
  next?.el.setAttribute(PANE_HIT_ATTR, '')
}

/**
 * The other half of what a pane hover has to say: not just WHICH pane, but
 * WHERE IN IT — the live quadrant preview `SplitDropOverlay` draws, so a
 * sidebar row being carried over a pane shows the shape of the split it is
 * about to make before the release, exactly as dragging an editor tab always
 * did.
 *
 * Deliberately the SAME channel the explorer's file drag uses (the drag
 * store's `hover` → `PaneContainer`'s narrow selector → `SplitDropOverlay`'s
 * `activeZoneOverride`) rather than a second preview of this hook's own: the zone math is already shared (`getPaneDropZoneFromRect`,
 * which `paneZone.hit` above calls and `resolveDropTarget` calls for tabs), so
 * anything new here would be a second renderer for one already-shared answer,
 * free to drift from it. A pane's element lives inside `WorkspaceHost`, not
 * the sidebar, so this store is also the only handle either drag has on it.
 *
 * The store carries a plain `{paneId, zone}` and dedups its own writes, so
 * this is safe to call on every resolved hover; `clearDropHover` tears it
 * down at the end of the drag.
 */
export function publishPaneZonePreview(hit: PaneZoneHit | null): void {
  setDropHover({ paneId: hit?.paneId ?? null, zone: hit?.zone ?? null })
}
