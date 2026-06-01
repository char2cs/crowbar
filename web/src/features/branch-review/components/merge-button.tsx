'use client'

import { useState } from 'react'
import { CaretDown } from '@phosphor-icons/react'
import { Button } from '@/components/ui/button'
import { Group } from '@/components/ui/group'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Label } from '@/components/ui/label'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import {
  Popover,
  PopoverTrigger,
  PopoverContent,
  PopoverClose,
} from '@/components/ui/popover'
import type { MergeStrategy } from '@/features/branch-review/types/review-types'

const STRATEGY_LABELS: Record<MergeStrategy, string> = {
  merge: 'Merge commit',
  squash: 'Squash and merge',
  rebase: 'Rebase and merge',
}

interface MergeButtonProps {
  strategy: MergeStrategy
  branchName?: string
  isLocked: boolean
  hasConflicts: boolean
  onMerge: (message: { title: string; description: string }) => void
  onStrategyChange: (strategy: MergeStrategy) => void
}

export function MergeButton({
  strategy,
  branchName,
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

  const variant = hasConflicts ? 'destructive' : 'default'

  const [open, setOpen] = useState(false)
  const [title, setTitle] = useState('')
  const [description, setDescription] = useState('')

  const defaultTitle = branchName ? `Merge ${branchName}` : STRATEGY_LABELS[strategy]

  function handleSubmit() {
    onMerge({ title: title.trim() || defaultTitle, description: description.trim() })
    setOpen(false)
  }

  return (
    <Group>
      <Popover open={open} onOpenChange={setOpen}>
        <PopoverTrigger
          disabled={disabled}
          render={
            <Button variant={variant} disabled={disabled} tooltip={tooltip} className="text-foreground" />
          }
        >
          {STRATEGY_LABELS[strategy]}
        </PopoverTrigger>
        <PopoverContent align="end" className="w-[440px]">
          <form
            className="flex flex-col gap-3"
            onSubmit={e => { e.preventDefault(); handleSubmit() }}
          >
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="merge-title">Commit message</Label>
              <Input
                id="merge-title"
                autoFocus
                value={title}
                onChange={e => setTitle(e.target.value)}
                placeholder={defaultTitle}
              />
            </div>
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="merge-description">Extended description</Label>
              <Textarea
                id="merge-description"
                rows={4}
                value={description}
                onChange={e => setDescription(e.target.value)}
                placeholder="Add an optional extended description…"
                className="resize-none"
              />
            </div>
            <div className="flex justify-end gap-2 pt-1">
              <PopoverClose
                render={<Button type="button" variant="outline" size="sm" />}
              >
                Cancel
              </PopoverClose>
              <Button type="submit" variant={variant} size="sm" className="text-foreground">
                {STRATEGY_LABELS[strategy]}
              </Button>
            </div>
          </form>
        </PopoverContent>
      </Popover>

      <DropdownMenu>
        <DropdownMenuTrigger
          disabled={disabled}
          render={<Button variant={variant} size="icon" disabled={disabled} className="text-foreground" />}
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
