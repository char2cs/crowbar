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
import { toast } from '@/components/ui/toast'
import type { MergeStrategy } from '@/features/branch-review/types/review-types'

export async function executeMerge(
  onMerge: (message: { title: string; description: string }) => void | Promise<void>,
  message: { title: string; description: string },
): Promise<void> {
  try {
    await onMerge(message)
    toast.success('Merged successfully')
  } catch {
    toast.error('Merge failed — check logs')
  }
}

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
  onMerge: (message: { title: string; description: string }) => void | Promise<void>
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
  const [merging, setMerging] = useState(false)

  const defaultTitle = branchName ? `Merge ${branchName}` : STRATEGY_LABELS[strategy]

  async function handleSubmit() {
    const message = { title: title.trim() || defaultTitle, description: description.trim() }
    setMerging(true)
    try {
      await executeMerge(onMerge, message)
    } finally {
      setMerging(false)
      setOpen(false)
    }
  }

  return (
    <Group>
      <Popover open={open} onOpenChange={setOpen}>
        <PopoverTrigger
          disabled={disabled}
          render={
            <Button variant={variant} disabled={disabled} tooltip={tooltip} className="text-primary-foreground" />
          }
        >
          {STRATEGY_LABELS[strategy]}
        </PopoverTrigger>
        <PopoverContent align="end" className="w-[440px]">
          <form
            className="flex flex-col gap-3"
            onSubmit={e => { e.preventDefault(); void handleSubmit() }}
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
              <Button type="submit" variant={variant} size="sm" className="text-primary-foreground" disabled={merging}>
                {merging ? 'Merging…' : STRATEGY_LABELS[strategy]}
              </Button>
            </div>
          </form>
        </PopoverContent>
      </Popover>

      <DropdownMenu>
        <DropdownMenuTrigger
          disabled={disabled}
          render={<Button variant={variant} size="icon" disabled={disabled} className="text-primary-foreground" />}
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
