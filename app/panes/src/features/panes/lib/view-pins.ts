import type { ViewState } from './view-state'

type Order = Pick<ViewState, 'views' | 'viewOrder'>

/** Pinned views lead the band, each group in its own order; a write that broke it is put right. */
export function settlePinnedOrder(state: Order): void {
  const pinned = state.viewOrder.filter((id) => state.views[id]?.pinned)
  const rest = state.viewOrder.filter((id) => !state.views[id]?.pinned)
  const next = [...pinned, ...rest]
  if (next.some((id, i) => id !== state.viewOrder[i])) state.viewOrder = next
}

/** Pin or unpin: the view joins the end of the pinned group, or the start of the rest. */
export function setViewPinned(state: Order, viewId: string, pinned: boolean): void {
  const view = state.views[viewId]
  if (!view || !!view.pinned === pinned) return
  if (pinned) view.pinned = true
  else delete view.pinned
  const rest = state.viewOrder.filter((id) => id !== viewId)
  const at = rest.filter((id) => state.views[id]?.pinned).length
  rest.splice(at, 0, viewId)
  state.viewOrder = rest
}
