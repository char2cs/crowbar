import type React from 'react'

interface SidebarToggleIconProps extends React.SVGProps<SVGSVGElement> {
  size?: number
}

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
export function SidebarToggleIcon({ size = 16, ...props }: SidebarToggleIconProps) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={2}
      strokeLinecap="round"
      strokeLinejoin="round"
      xmlns="http://www.w3.org/2000/svg"
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
