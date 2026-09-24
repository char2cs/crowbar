import { useMemo, type CSSProperties } from 'react'
import type { PanePosition } from '../types/pane'
import { buildPaneContentStyle } from '../utils/pane-border'

interface PaneAccentRingProps {
  position: PanePosition
  sidebarPosition: 'left' | 'right'
  sidebarOpen: boolean
  visible: boolean
}

/**
 * The active-pane accent: the border `buildPaneContentStyle` would put on the
 * pane box when active, lifted onto a childless overlay over that box's border
 * box (its margins become insets) and faded by OPACITY. Fading a colour on the
 * large, rounded, translucent pane box itself repaints all of it every frame
 * (measured: 85 fps vs 116 fps over 12 focus clicks); the compositor fades an
 * overlay without repainting anything under it.
 */
export function PaneAccentRing({
  position,
  sidebarPosition,
  sidebarOpen,
  visible,
}: PaneAccentRingProps) {
  const ringStyle = useMemo<CSSProperties>(() => {
    const accent = buildPaneContentStyle(position, sidebarPosition, true, sidebarOpen)
    return {
      position: 'absolute',
      left: accent.marginLeft,
      top: accent.marginTop,
      right: accent.marginRight,
      bottom: accent.marginBottom,
      borderTop: accent.borderTop,
      borderLeft: accent.borderLeft,
      borderRight: accent.borderRight,
      borderBottom: accent.borderBottom,
      borderTopLeftRadius: accent.borderTopLeftRadius,
      borderTopRightRadius: accent.borderTopRightRadius,
      borderBottomLeftRadius: accent.borderBottomLeftRadius,
      borderBottomRightRadius: accent.borderBottomRightRadius,
    }
  }, [position, sidebarPosition, sidebarOpen])

  return (
    <div
      data-pane-accent=""
      aria-hidden="true"
      className="pointer-events-none absolute z-[2] transition-opacity duration-150"
      style={{ ...ringStyle, opacity: visible ? 1 : 0 }}
    />
  )
}
