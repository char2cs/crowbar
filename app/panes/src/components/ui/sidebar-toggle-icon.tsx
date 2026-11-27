import type React from 'react'
import { cn } from '@/utils/cn'

type SidebarToggleIconProps = React.SVGProps<SVGSVGElement>

export function SidebarToggleIcon({ className, ...props }: SidebarToggleIconProps) {
  return (
    <svg
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
      <rect x="3" y="4" width="18" height="16" rx="2.5" />
      <path d="M9 4v16" />
      <path d="M5.5 9h1.5" />
      <path d="M5.5 13h1.5" />
    </svg>
  )
}
