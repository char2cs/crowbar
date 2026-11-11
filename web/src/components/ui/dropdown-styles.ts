import { cva } from 'class-variance-authority'
import { buttonVariants } from '@/components/ui/button-variants'
import { cn } from '@/utils/cn'

// `ghost` is for triggers on a surface; with no box, its optical edge is the icon, so text-column
// alignment pulls back px-2 PLUS the 1px transparent border (9px). Variants are cn()'d so the
// trigger's `px-2` wins; cva's base string skips tailwind-merge and would lose to buttonVariants.
const TRIGGER_SHARED = 'min-w-0 gap-1 rounded-lg px-2 text-muted-foreground'

const dropdownTriggerVariants = cva('', {
  variants: {
    variant: {
      default: cn(buttonVariants({ variant: 'default' }), TRIGGER_SHARED),
      ghost: cn(buttonVariants({ variant: 'ghost' }), TRIGGER_SHARED),
    },
  },
  defaultVariants: {
    variant: 'default',
  },
})

export type DropdownTriggerVariant = 'default' | 'ghost'

export const dropdownRootVariants = cva(
  'pointer-events-auto fixed z-[10040] min-w-[240px] max-w-[min(480px,calc(100vw-16px))] select-none overflow-y-auto rounded-xl border border-border bg-card/95 p-1 shadow-[0_14px_30px_-24px_rgba(0,0,0,0.45)] backdrop-blur-sm [overscroll-behavior:contain]',
)

export const dropdownItemVariants = cva(
  'ui-font ui-text-sm flex w-full items-center justify-between gap-3 whitespace-nowrap rounded-lg px-2.5 py-1.5 text-left text-foreground transition-colors',
  {
    variants: {
      disabled: {
        true: 'cursor-not-allowed opacity-50',
        false: 'cursor-pointer hover:bg-muted',
      },
      focused: {
        true: 'bg-muted',
        false: '',
      },
    },
    defaultVariants: {
      disabled: false,
      focused: false,
    },
  },
)

export const dropdownSectionLabelVariants = cva(
  'ui-font ui-text-sm px-2.5 py-1 text-muted-foreground',
)

const DROPDOWN_ITEM_BASE = dropdownItemVariants()

export function dropdownTriggerClassName(
  className?: string,
  variant: DropdownTriggerVariant = 'default',
) {
  return cn(dropdownTriggerVariants({ variant }), className)
}

export function dropdownItemClassName(className?: string) {
  return cn(DROPDOWN_ITEM_BASE, className)
}
