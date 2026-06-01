'use client'

import { Button } from '@/components/ui/button'
import { Group } from '@/components/ui/group'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { CaretDown } from '@phosphor-icons/react'
import type { MergeStrategy } from '@/features/branch-review/types/review-types'
import { cn } from '@/utils/cn'

const STRATEGY_LABELS: Record<MergeStrategy, string> = {
  merge: 'Merge commit',
  squash: 'Squash and merge',
  rebase: 'Rebase and merge',
}

interface MergeButtonProps {
  strategy: MergeStrategy
  isLocked: boolean
  hasConflicts: boolean
  onMerge: () => void
  onStrategyChange: (strategy: MergeStrategy) => void
}

export function MergeButton({
  strategy,
  isLocked,
  hasConflicts,
  onMerge,
  onStrategyChange,
}: MergeButtonProps) {
  const disabled = isLocked || hasConflicts
  const tooltip = isLocked
    ? 'Cannot merge into a locked branch'
    : hasConflicts
      ? 'Branch has conflicts with parent'
      : undefined

  return (
    <Group>
      <Button
        variant={hasConflicts ? 'destructive' : 'default'}
        disabled={disabled}
        onClick={onMerge}
        title={tooltip}
        className={cn(!disabled && 'bg-green-700 hover:bg-green-600 text-white')}
      >
        {STRATEGY_LABELS[strategy]}
      </Button>
      <DropdownMenu>
        <DropdownMenuTrigger
          disabled={disabled}
          title={tooltip}
          className={cn(
            'relative inline-flex shrink-0 cursor-pointer items-center justify-center gap-2 whitespace-nowrap rounded-lg border font-medium text-base outline-none transition-shadow px-2',
            'h-9 sm:h-8',
            'focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-1',
            'disabled:pointer-events-none disabled:opacity-64',
            hasConflicts
              ? 'border-destructive bg-destructive text-white hover:bg-destructive/90'
              : disabled
                ? 'border-primary bg-primary text-primary-foreground'
                : 'border-primary bg-green-800 hover:bg-green-700 text-green-200',
          )}
        >
          <CaretDown size={12} />
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          {(Object.keys(STRATEGY_LABELS) as MergeStrategy[]).map((s) => (
            <DropdownMenuItem key={s} onSelect={() => onStrategyChange(s)}>
              {STRATEGY_LABELS[s]}
            </DropdownMenuItem>
          ))}
        </DropdownMenuContent>
      </DropdownMenu>
    </Group>
  )
}
