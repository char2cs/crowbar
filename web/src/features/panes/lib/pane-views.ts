import type { LayoutNode, PaneGroup } from '@/features/panes/types/pane'
import { closeLayout, getAllLeafIds, normalizeLayout } from '@/features/panes/utils/pane-layout'

/** The minimum a pane has to carry to answer which view it is in. Kept
 *  structural so a caller holding a persisted/partial pane record (or a test
 *  fixture) can ask without constructing a whole `PaneGroup`. */
type ViewTagged = Pick<PaneGroup, 'id'> & Partial<Pick<PaneGroup, 'viewId'>>

/**
 * Which VIEW `pane` belongs to.
 *
 * The one read path for `PaneGroup.viewId`, because the field is
 * deliberately optional and its absence MEANS something: an untagged pane is
 * its own view, identified by its own id. That covers two cases with one
 * rule — a layout persisted before views existed (no migration, just a
 * correct reading of the old shape: every pane was independent), and any
 * pane a future code path forgets to tag, which degrades to "independent"
 * rather than to "silently joined whatever group it sits beside".
 */
export function viewIdOf(pane: ViewTagged): string {
  return pane.viewId ?? pane.id
}

/** Every pane sharing `pane`'s view, `pane` included — the group as it
 *  actually stands right now. A pane nobody merged with answers `[pane]`,
 *  which is exactly why a view dissolving to one member needs no special
 *  case anywhere: a group of one and an ungrouped pane are the same thing. */
export function panesInView(panes: Record<string, PaneGroup>, paneId: string): PaneGroup[] {
  const pane = panes[paneId]
  if (!pane) return []
  const view = viewIdOf(pane)
  return Object.values(panes).filter((p) => viewIdOf(p) === view)
}

/** Whether any pane OTHER than `paneId` is in the same view — i.e. whether
 *  `paneId` is one member of a genuinely merged view rather than a view of
 *  its own. */
export function viewIsShared(panes: Record<string, PaneGroup>, paneId: string): boolean {
  const pane = panes[paneId]
  if (!pane) return false
  const view = viewIdOf(pane)
  return Object.values(panes).some((p) => p.id !== paneId && viewIdOf(p) === view)
}

/**
 * Split ONE tiling tree that holds several views into one tree PER view.
 *
 * A view owns its own tree in the store (`PaneSlice.rootLayout` for the
 * showing one, `parkedViews[viewId]` for the rest), so nothing has to filter
 * views out of a shared tree at render time. This is the one place that has
 * to deal with a tree that mixes them, and it exists for exactly one input:
 * a layout persisted by a build in which every view was tiled into
 * `rootLayout` together. Reading that old shape as "one view per `viewId`,
 * each with its own tree" is the graceful fallback — without it the first
 * reload after this feature ships would faithfully restore the very
 * side-by-side tiling the feature removes.
 *
 * Built by SUBTRACTION (`closeLayout` per foreign leaf) rather than by
 * constructing fresh splits, so each view keeps the real proportions and
 * nesting its panes already had relative to one another instead of being
 * re-tiled into arbitrary halves.
 */
export function partitionLayoutByView(
  layout: LayoutNode,
  panes: Record<string, PaneGroup>,
): Record<string, LayoutNode> {
  const leafIds = getAllLeafIds(layout)
  const members = new Map<string, Set<string>>()
  for (const id of leafIds) {
    const view = viewIdOf(panes[id] ?? { id })
    const bucket = members.get(view)
    if (bucket) bucket.add(id)
    else members.set(view, new Set([id]))
  }

  const trees: Record<string, LayoutNode> = {}
  for (const [viewId, keep] of members) {
    let tree: LayoutNode | null = layout
    for (const id of leafIds) {
      if (keep.has(id) || tree === null) continue
      tree = closeLayout(tree, id)
    }
    if (tree !== null) trees[viewId] = normalizeLayout(tree)
  }
  return trees
}
