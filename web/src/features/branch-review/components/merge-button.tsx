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

const SUCCESS_CLASSES = 'bg-[var(--success)] hover:bg-[var(--success)]/90 border-[var(--success)] text-white'

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

  return (
    <Group>
      <Button
        variant={hasConflicts ? 'destructive' : 'default'}
        disabled={disabled}
        onClick={onMerge}
        tooltip={tooltip}
        className={!disabled && !hasConflicts ? SUCCESS_CLASSES : undefined}
      >
        {STRATEGY_LABELS[strategy]}
      </Button>
      <DropdownMenu>
        <DropdownMenuTrigger
          disabled={disabled}
          render={
            <Button
              variant={hasConflicts ? 'destructive' : 'default'}
              size="icon-sm"
              disabled={disabled}
              className={!disabled && !hasConflicts ? SUCCESS_CLASSES : undefined}
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
