import { SquareSplitHorizontal } from '@phosphor-icons/react'
import { Button } from '@/components/ui/button'
import { cn } from '@/utils/cn'

interface SplitToggleButtonProps {
  active: boolean
  onToggle: () => void
}

export function SplitToggleButton({ active, onToggle }: SplitToggleButtonProps) {
  return (
    <Button
      variant="ghost"
      size="icon-sm"
      data-testid="split-toggle"
      data-role="split-toggle"
      aria-pressed={active}
      aria-label={active ? 'Show chat only' : 'Show editor alongside chat'}
      onClick={onToggle}
      tooltip={active ? 'Show chat only' : 'Show editor alongside chat'}
      tooltipSide="bottom"
      className={cn(
        'shrink-0 rounded-sm text-muted-foreground hover:bg-sidebar-element-hover',
        active && 'text-foreground',
      )}
    >
      <SquareSplitHorizontal />
    </Button>
  )
}
