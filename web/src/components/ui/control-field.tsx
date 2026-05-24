// Stub UI component - provides Athas control-field compatibility
import type { ReactNode } from 'react'
import { cva } from "class-variance-authority"

export const controlFieldIconSizes = {
  xs: "size-3",
  sm: "size-3.5",
  md: "size-4",
  lg: "size-5",
} as const

export const controlFieldSizeVariants = cva("", {
  variants: {
    size: {
      xs: "h-6 text-xs",
      sm: "h-7 text-sm",
      md: "h-8 text-sm",
      lg: "h-9 text-base",
    },
  },
  defaultVariants: { size: "md" },
})

export const controlFieldSurfaceVariants = cva("", {
  variants: {
    surface: {
      default: "border border-input bg-background",
      ghost: "border-transparent bg-transparent",
      outline: "border border-border bg-transparent",
    },
    variant: {
      default: "border border-input bg-background",
      secondary: "border border-border bg-muted",
      ghost: "border-transparent bg-transparent",
      outline: "border border-border bg-transparent",
    },
  },
  defaultVariants: { surface: "default" },
})

export interface ControlFieldProps {
  children?: ReactNode
  className?: string
  size?: "xs" | "sm" | "md" | "lg"
  surface?: "default" | "ghost" | "outline"
}

export function ControlField({ children }: ControlFieldProps) {
  return <>{children}</>
}

export default ControlField
