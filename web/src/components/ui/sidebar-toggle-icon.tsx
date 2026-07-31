import type React from 'react'
import { cn } from '@/utils/cn'

type SidebarToggleIconProps = React.SVGProps<SVGSVGElement>

/**
 * Sidebar-toggle glyph: a rounded panel with a left rail that carries two short
 * content lines. Lucide's PanelLeft leaves that rail empty, which reads as a
 * plain divided rectangle rather than "a sidebar"; the content lines are what
 * make the metaphor legible, matching the reference browser's toolbar.
 *
 * Authored here rather than copied: the reference icon is from a commercial
 * (Nucleo) set that cannot be redistributed. This is original path data for the
 * same standard UI metaphor. Stroke-based so it composes with the muted token
 * and sits consistently beside the Lucide arrows/settings in the same cluster.
 */
export function SidebarToggleIcon({ className, ...props }: SidebarToggleIconProps) {
  return (
    <svg
      // Sized by CLASS, not by width/height attributes, and that is the whole
      // point. Button sizes its icons with `[&_svg:not([class*='size-'])]`,
      // which beats presentation attributes — so the old `size={14}` / `size={16}`
      // props were silently ignored and the glyph took whichever size its button
      // variant dictated: 14px in the tab bar (icon-xs), 16px in the sidebar
      // header (icon-sm), and 16/18px respectively below the `sm` breakpoint.
      // Same button, four sizes. Carrying a size- class opts out of that rule
      // entirely, so the toggle is one size everywhere; callers can still
      // override by passing their own.
      className={cn('size-4', className)}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={2}
      strokeLinecap="round"
      strokeLinejoin="round"
      xmlns="http://www.w3.org/2000/svg"
      // Neither declaration, and both for the same reason as the box above:
      // `size-4` *authors* 16px on both axes, so the width is not a content
      // width (`native/oracle/ANCHORS.md` v1.5) and the height is not a line box
      // (v1.6 — this element paints no text at all). The stroke is invisible to
      // the contract: `stroke`/`stroke-width` have no field, and `fg` is emitted
      // only for an element with its own text nodes. The anchor pins the box.
      data-oracle-id="sidebar-toggle-icon"
      {...props}
    >
      {/* panel */}
      <rect x="3" y="4" width="18" height="16" rx="2.5" />
      {/* divider — left rail is ~30% of the width */}
      <path d="M9 4v16" />
      {/* content lines in the left rail */}
      <path d="M5.5 9h1.5" />
      <path d="M5.5 13h1.5" />
    </svg>
  )
}
