import type { ReactNode } from 'react'

interface CollapseSectionProps {
  open: boolean
  role?: string
  className?: string
  children: ReactNode
}

/**
 * A collapsible section of the workspace tree.
 *
 * This is deliberately an immediate mount/unmount. Animating `height` forced
 * WebKit to lay out the entire sidebar on every frame of the tween; a three-row
 * repo was enough to miss several frames, and nested sections multiplied that
 * work. A disclosure control is navigation chrome, so responsiveness wins over
 * a decorative transition here.
 */
export function CollapseSection({ open, role, className, children }: CollapseSectionProps) {
  if (!open) return null

  return (
    <div role={role} className={className} data-collapse-section="open">
      {children}
    </div>
  )
}
