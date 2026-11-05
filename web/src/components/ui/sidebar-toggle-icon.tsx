import type React from 'react'
import { cn } from '@/utils/cn'

type SidebarToggleIconProps = React.SVGProps<SVGSVGElement>

// A panel whose left rail carries two content lines (an empty rail reads as a divided box, not a
// sidebar). Original path data: the reference is a non-redistributable commercial icon. Stroked to
// sit with the Lucide glyphs beside it.
export function SidebarToggleIcon({ className, ...props }: SidebarToggleIconProps) {
  return (
    <svg
      // Sized by CLASS: Button's `[&_svg:not([class*='size-'])]` rule beats width/height attributes,
      // so a size- class is what keeps the toggle one size everywhere (callers may override).
      className={cn('size-4', className)}
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
