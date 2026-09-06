import type { PaneGroup } from '@/features/panes/types/pane'

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
