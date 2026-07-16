import type { CSSProperties } from 'react'

/**
 * Slot styling for one workspace surface in WorkspaceHost.
 *
 * A "slot" is the wrapper `<div>` around each retained WorkspaceView. Inactive
 * (retained-but-hidden) workspaces are dropped from the render tree with
 * `display:none` + `inert`, so only the active workspace paints; switching back
 * re-inserts the subtree from its still-live store (no re-hydrate). The active
 * slot is `display:contents` — a transparent box — so the workspace's own layout
 * root flexes directly inside the content pane.
 *
 * DO NOT swap `display:none` for `content-visibility:hidden` here. It was tried
 * (to shave the reveal cost) and REVERTED: with up to RETENTION_CAP workspaces
 * retained at once, a `content-visibility:hidden` box plus a
 * `contain-intrinsic-size` placeholder left every hidden slot's Monaco/xterm
 * believing it still had a full-size viewport, so they kept laying out and
 * rendering while hidden — several live workspaces pinning the CPU (fans
 * spinning), plus editor panes that failed to paint on reveal. `display:none`
 * collapses hidden widgets to zero size so they go dormant; that dormancy is
 * load-bearing, not incidental.
 *
 * Pure function, so the (trivial) hide decision is unit-testable without a DOM.
 */
export interface WorkspaceSlotStyling {
  style: CSSProperties
  /** `inert` keeps DOM focus/pointer/AT out of a hidden retained workspace. */
  inert: boolean
}

export function workspaceSlotStyling(isActive: boolean): WorkspaceSlotStyling {
  return {
    style: { display: isActive ? 'contents' : 'none' },
    inert: !isActive,
  }
}
