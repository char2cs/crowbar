import type { CSSProperties } from 'react'

export interface WorkspaceSlotStyling {
  style: CSSProperties
  inert: boolean
}

export function workspaceSlotStyling(isActive: boolean): WorkspaceSlotStyling {
  return {
    style: { display: isActive ? 'contents' : 'none' },
    inert: !isActive,
  }
}
