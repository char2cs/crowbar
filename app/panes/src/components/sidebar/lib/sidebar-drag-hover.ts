import { dropLineBox, dropLineBoxX } from '@/components/layout/drop-indicator'
import { anyModeAllowed, NO_MODES, type DropPolicy } from '@/components/tree-dnd/drop-core'
import {
  createDropHitTest,
  createDropRowDom,
  type DropRowSpec,
  type DropZone,
} from '@/components/tree-dnd/drop-dom'
import { getPaneDropZoneFromRect, type PaneDropZone } from '@/features/panes/utils/pane-drop-zones'
import { setDropHover } from '@/features/panes/stores/drag-store'
import { SIDEBAR_DROP_POLICY } from '@/components/sidebar/lib/sidebar-drop-policy'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'

/** Facts a row's own tree-walk knows that `SidebarRow` itself does not carry. */
export interface RowDragExtra {
  /** The ancestor chain, delimited on both sides (`/root/a/`), for the cycle guard;
   *  absent reads as no known ancestry (a Recents entry), which allows every drop. */
  path?: string
  expanded?: boolean
  hasChildren?: boolean
  /** Published only by a Recents tab (`view-row.tsx`): spec §8.1 gives the same geometry over a Recents
   *  entry another meaning than over a tree row, and `SidebarRow` cannot tell them apart. */
  inRecents?: boolean
}

export type DragRow = SidebarRow & RowDragExtra

/** One attribute per row kind, so a hit test tells a row apart with one sweep. */
const SIDEBAR_DRAG_ROW_SPEC: DropRowSpec = {
  kinds: {
    branch: 'data-sidebar-branch-drop',
    folder: 'data-sidebar-folder-drop',
    chat: 'data-sidebar-chat-drop',
    workflow: 'data-sidebar-workflow-drop',
  },
  parentAttr: 'data-sidebar-drop-parent',
  strings: { path: 'data-sidebar-path' },
  flags: {
    expanded: 'data-sidebar-expanded',
    hasChildren: 'data-sidebar-children',
    inRecents: 'data-sidebar-recents-row',
  },
}

export const rowDom = createDropRowDom<DragRow>(SIDEBAR_DRAG_ROW_SPEC)

/** A row may not drop into its own descendant: the matrix never reads ancestry, so the
 *  row's published chain is tested for each subject's id (delimited, so `/ab/` never
 *  matches `a`). */
export function cycleSafeAllowedModes(subjects: readonly DragRow[], target: DragRow) {
  const allowed = SIDEBAR_DROP_POLICY.allowedModes(subjects, target)
  if (!anyModeAllowed(allowed)) return allowed
  const path = target.path ?? ''
  return subjects.some((s) => path.includes(`/${s.id}/`)) ? NO_MODES : allowed
}

const SIDEBAR_DRAG_POLICY: DropPolicy<DragRow, DragRow> = {
  allowedModes: cycleSafeAllowedModes,
  edgeBandFor: SIDEBAR_DROP_POLICY.edgeBandFor,
}

/** The zones a pane resolves to (spec §8.1): center opens into it, an edge splits it. */
export type SidebarPaneZone = 'center' | 'left' | 'right' | 'top' | 'bottom'

export interface SidebarPaneHit {
  kind: 'pane'
  paneId: string
  zone: SidebarPaneZone
}

/** A droppable pane element carries this, valued with its pane id. */
export const PANE_DROP_ATTR = 'data-pane-drop'

/** Painted onto the pane element a release would land in, for its CSS ring (spec §8.2). */
const PANE_HIT_ATTR = 'data-pane-hit'

/** The pane hit plus the element `elementsFromPoint` resolved: pane ids repeat across the
 *  retained (hidden) workspaces, so only the painted element names the right node. */
export interface ResolvedPaneHit extends SidebarPaneHit {
  el: Element
}

const paneZone: DropZone<DragRow, ResolvedPaneHit> = {
  attr: PANE_DROP_ATTR,
  hit: (_subjects, el, point) => {
    const paneId = el.getAttribute(PANE_DROP_ATTR)
    if (!paneId) return null
    const rect = el.getBoundingClientRect()
    if (rect.width <= 0 || rect.height <= 0) return null
    const zone: PaneDropZone = getPaneDropZoneFromRect(point, rect)
    return { kind: 'pane', paneId, zone: zone ?? 'center', el }
  },
}

export const hitTest = createDropHitTest<DragRow, DragRow, ResolvedPaneHit>(
  rowDom,
  SIDEBAR_DRAG_POLICY,
  [paneZone],
)

export type Hit = ReturnType<typeof hitTest>

/** Moves the hairline to `hit`'s slot, writing only the styles that changed. */
export function paintDropLine(el: HTMLDivElement | null, hit: Hit): void {
  if (!el) return
  if (hit?.kind !== 'row' || hit.row.mode === 'into') {
    el.style.display = 'none'
    return
  }
  const box =
    hit.axis === 'x' ? dropLineBoxX(hit.rect, hit.row.mode) : dropLineBox(hit.rect, hit.row.mode)
  el.style.display = 'block'
  if (el.dataset.dropIndicator !== hit.row.mode) el.dataset.dropIndicator = hit.row.mode
  const height = 'height' in box ? `${box.height}px` : ''
  if (el.style.height !== height) el.style.height = height
  const left = `${box.left}px`
  const top = `${box.top}px`
  const width = `${box.width}px`
  if (el.style.left !== left) el.style.left = left
  if (el.style.top !== top) el.style.top = top
  if (el.style.width !== width) el.style.width = width
}

/** While the list edge-scrolls the line tracks it, so its transition would only trail. */
export function setDropLineTracking(el: HTMLDivElement | null, tracking: boolean): void {
  if (!el) return
  el.style.transition = tracking ? 'none' : ''
}

export function samePaneHit(a: SidebarPaneHit | null, b: SidebarPaneHit | null): boolean {
  if (a === null || b === null) return a === b
  return a.paneId === b.paneId && a.zone === b.zone
}

/** Moves the pane ring to `next`'s element. Gated on the element, not the pane id: two
 *  elements share an id, and a center-to-edge move on one pane keeps its ring. */
export function paintPaneHit(prev: ResolvedPaneHit | null, next: ResolvedPaneHit | null): void {
  if ((prev?.el ?? null) === (next?.el ?? null)) return
  prev?.el.removeAttribute(PANE_HIT_ATTR)
  next?.el.setAttribute(PANE_HIT_ATTR, '')
}

/** The split preview goes through the drag store, the channel a file drag already uses:
 *  the pane lives outside the sidebar, and the store dedups its own writes. */
export function publishPaneZonePreview(hit: SidebarPaneHit | null): void {
  setDropHover({ paneId: hit?.paneId ?? null, zone: hit?.zone ?? null })
}
