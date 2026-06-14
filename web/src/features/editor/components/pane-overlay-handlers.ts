import type React from 'react'

/**
 * Container-level mouse handlers for the editor overlay surface (LSP hover,
 * cmd-hover definition link, cmd-click go-to-definition).
 *
 * {@link EditorSurface} attaches STABLE wrapper handlers to the container div
 * and forwards each event to the latest set published here by
 * {@link PaneLspLayer} — so the LSP layer can re-render on a buffer switch
 * without changing the container's handler identities (which would re-render
 * EditorSurface).
 */
export interface PaneOverlayMouseHandlers {
  handleMouseMove: (e: React.MouseEvent<HTMLDivElement>) => void
  handleMouseLeave: () => void
  handleMouseEnter: () => void
  handleClick: (e: React.MouseEvent<HTMLDivElement>) => void
}
