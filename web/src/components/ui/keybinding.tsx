import { cva } from 'class-variance-authority'
import { cn } from '@/utils/cn'
import { keybindingToDisplay } from '@/utils/keybinding-display'
import { IS_MAC } from '@/utils/platform'

interface KeybindingProps {
  keys?: string[]
  binding?: string
  className?: string
  'data-oracle-id'?: string
}

const keybindingKeyVariants = cva(
  'ui-font ui-text-sm inline-flex min-h-4 items-center justify-center rounded-md border border-border bg-card px-1.5 leading-none text-muted-foreground shadow-[inset_0_-1px_0_rgba(0,0,0,0.12)]',
)

export default function Keybinding({
  keys,
  binding,
  className,
  'data-oracle-id': oracleId = 'keybinding',
}: KeybindingProps) {
  const displayKeys = binding ? keybindingToDisplay(binding) : (keys ?? [])

  if (displayKeys.length === 0) {
    return null
  }

  return (
    <kbd
      className={cn(keybindingKeyVariants(), className)}
      data-oracle-content-sized
      data-oracle-id={oracleId}
    >
      {displayKeys.join(IS_MAC ? '' : '+')}
    </kbd>
  )
}
