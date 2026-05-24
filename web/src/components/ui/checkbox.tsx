import { Checkbox as CheckboxPrimitive } from "@base-ui/react/checkbox"
import type React from "react"

import { cn } from "@/lib/utils"
import { CheckIcon } from "lucide-react"

type CheckboxProps = CheckboxPrimitive.Root.Props & {
  /** Athas compatibility: onChange fires with the new checked boolean */
  onChange?: ((checked: boolean) => void) | React.Dispatch<React.SetStateAction<boolean>>
  /** Athas compatibility: aria-label shorthand */
  ariaLabel?: string
}

function Checkbox({ className, onChange, onCheckedChange, ariaLabel, ...props }: CheckboxProps) {
  const handleCheckedChange = onCheckedChange ?? (onChange ? (checked: boolean) => onChange(checked) : undefined)
  return (
    <CheckboxPrimitive.Root
      data-slot="checkbox"
      aria-label={ariaLabel}
      className={cn(
        "peer relative flex size-4 shrink-0 items-center justify-center rounded-[4px] border border-input transition-colors outline-none group-has-disabled/field:opacity-50 after:absolute after:-inset-x-3 after:-inset-y-2 focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 disabled:cursor-not-allowed disabled:opacity-50 aria-invalid:border-destructive aria-invalid:ring-3 aria-invalid:ring-destructive/20 aria-invalid:aria-checked:border-primary dark:bg-input/30 dark:aria-invalid:border-destructive/50 dark:aria-invalid:ring-destructive/40 data-checked:border-primary data-checked:bg-primary data-checked:text-primary-foreground dark:data-checked:bg-primary",
        className
      )}
      onCheckedChange={handleCheckedChange}
      {...props}
    >
      <CheckboxPrimitive.Indicator
        data-slot="checkbox-indicator"
        className="grid place-content-center text-current transition-none [&>svg]:size-3.5"
      >
        <CheckIcon
        />
      </CheckboxPrimitive.Indicator>
    </CheckboxPrimitive.Root>
  )
}

export { Checkbox }
export default Checkbox
