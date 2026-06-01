'use client'

import { CaretDown } from '@phosphor-icons/react'
import { Button } from '@/components/ui/button'
import { Group } from '@/components/ui/group'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import type { MergeStrategy } from '@/features/branch-review/types/review-types'

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

export function MergeButton({ strategy, isLocked, hasConflicts, onMerge, onStrategyChange }: MergeButtonProps) {
  const disabled = isLocked || hasConflicts
  const tooltip = isLocked
    ? 'Cannot merge into a locked branch'
    : hasConflicts
      ? 'Branch has conflicts with parent'
      : undefined

  const variant = hasConflicts ? 'destructive' : 'default'

  return (
    <Group>
      <Button
        variant={variant}
        disabled={disabled}
        onClick={onMerge}
        tooltip={tooltip}
        className="text-foreground"
      >
        {STRATEGY_LABELS[strategy]}
      </Button>
      <DropdownMenu>
        <DropdownMenuTrigger
          disabled={disabled}
          render={
            <Button
              variant={variant}
              size="icon"
              disabled={disabled}
              className="text-foreground"
            />
          }
        >
          <CaretDown size={12} />
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          {(Object.keys(STRATEGY_LABELS) as MergeStrategy[]).map(s => (
            <DropdownMenuItem key={s} onClick={() => onStrategyChange(s)}>
              {STRATEGY_LABELS[s]}
            </DropdownMenuItem>
          ))}
        </DropdownMenuContent>
      </DropdownMenu>
    </Group>
  )
}
