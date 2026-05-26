import * as React from "react"
import { Input as InputPrimitive } from "@base-ui/react/input"
import type { Icon } from "@phosphor-icons/react"

import { cn } from "@/lib/utils"

export interface InputProps extends Omit<React.ComponentProps<"input">, "size"> {
  /** Icon component displayed on the left side (Athas compatibility) */
  leftIcon?: Icon | React.ComponentType<{ className?: string }>
  /** Visual variant (Athas compatibility, ignored at runtime) */
  variant?: "default" | "ghost" | "outline"
  /** Size variant */
  size?: "xs" | "sm" | "md" | "lg"
  /** Container class (Athas compatibility) */
  containerClassName?: string
}

function Input({ className, type, leftIcon: LeftIcon, variant: _variant, size: _size, containerClassName: _containerClassName, ...props }: InputProps) {
  return (
    <InputPrimitive
      type={type}
      data-slot="input"
      className={cn(
        "h-8 w-full min-w-0 rounded-lg border border-input bg-transparent px-2.5 py-1 text-base transition-colors outline-none file:inline-flex file:h-6 file:border-0 file:bg-transparent file:text-sm file:font-medium file:text-foreground placeholder:text-muted-foreground focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 disabled:pointer-events-none disabled:cursor-not-allowed disabled:bg-input/50 disabled:opacity-50 aria-invalid:border-destructive aria-invalid:ring-3 aria-invalid:ring-destructive/20 md:text-sm dark:bg-input/30 dark:disabled:bg-input/80 dark:aria-invalid:border-destructive/50 dark:aria-invalid:ring-destructive/40",
        LeftIcon && "pl-8",
        className
      )}
      {...props}
    />
  )
}

export { Input }
export default Input
