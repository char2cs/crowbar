import { useMemo, type CSSProperties } from 'react'
import type { PanePosition } from '../types/pane'
import { buildPaneContentStyle } from '../utils/pane-border'

interface PaneAccentRingProps {
  position: PanePosition
  sidebarPosition: 'left' | 'right'
  sidebarOpen: boolean
  visible: boolean
}

// The active-pane border on a childless overlay faded by opacity: fading a colour on the large
// translucent pane box repaints all of it each frame (85 vs 116 fps), while the compositor fades
// an overlay without repainting beneath it.
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
