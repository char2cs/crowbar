import type { CSSProperties } from 'react'

// Active slot is display:contents (its layout root flexes in the content pane); hidden ones are
// display:none, not content-visibility:hidden: zero size is what makes hidden Monaco/xterm go
// dormant instead of laying out and rendering off-screen.
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
