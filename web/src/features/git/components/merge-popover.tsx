import type { ReactElement } from 'react'
import { useState } from 'react'
import { Popover, PopoverTrigger, PopoverContent, PopoverTitle } from '@/components/ui/popover'
import { Button } from '@/components/ui/button'
import { RadioGroup, Radio } from '@/components/ui/radio-group'
import { toast } from '@/features/window/stores/toast-store'
import { useWorkspaceStoreById } from '@/features/workspace/stores/hooks/use-workspace-store-by-id'
import { getOrCreateWorkspaceStore } from '@/features/workspace/stores/workspace-store-registry'
import { setMergeStrategy as patchMergeStrategy, mergeIntoParent } from '../api/review-api'
import type { MergeStrategy } from '@/features/workspace/stores/slices/branch-review-slice'

const STRATEGIES: { value: MergeStrategy; label: string; confirm: string; desc: string }[] = [
  {
    value: 'merge',
    label: 'Create a merge commit',
    confirm: 'Create merge commit',
    desc: 'All commits, plus a merge commit',
  },
  { value: 'squash', label: 'Squash and merge', confirm: 'Squash & merge', desc: 'One combined commit' },
  {
    value: 'rebase',
    label: 'Rebase and merge',
    confirm: 'Rebase & merge',
    desc: 'Replay commits, no merge commit',
  },
]

interface MergePopoverProps {
  wsId: string
  parentBranch: string
  /** The "Merge into …" button rendered as the popover trigger. */
  trigger: ReactElement
}

export function MergePopover({ wsId, parentBranch, trigger }: MergePopoverProps) {
  const [open, setOpen] = useState(false)
  const strategy = useWorkspaceStoreById(wsId, (s) => s.branchReview.mergeStrategy)
  const active = STRATEGIES.find((s) => s.value === strategy) ?? STRATEGIES[0]

  // Persist the chosen strategy so it becomes the default next time (optimistic).
  const selectStrategy = async (next: MergeStrategy) => {
    if (next === strategy) return
    const previous = strategy
    getOrCreateWorkspaceStore(wsId).getState().setBranchReviewMergeStrategy(next)
    try {
      await patchMergeStrategy(wsId, next)
    } catch {
      getOrCreateWorkspaceStore(wsId).getState().setBranchReviewMergeStrategy(previous)
      toast.error('Failed to update merge strategy')
    }
  }

  const handleMerge = async () => {
    setOpen(false)
    try {
      await mergeIntoParent(wsId, strategy)
      toast.info('Merging…')
    } catch {
      toast.error('Merge failed — check the logs for details')
    }
  }

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger render={trigger} />
      <PopoverContent align="start" className="w-64">
        <PopoverTitle className="text-sm">Merge into {parentBranch}</PopoverTitle>
        <p className="ui-text-xs mt-0.5 mb-3 text-muted-foreground">
          {parentBranch} is local &amp; unprotected
        </p>
        <RadioGroup
          value={strategy}
          onValueChange={(value) => void selectStrategy(value as MergeStrategy)}
          className="mb-3 gap-1"
        >
          {STRATEGIES.map((s) => (
            <label key={s.value} className="flex cursor-pointer items-start gap-2 rounded-md py-0.5">
              <Radio value={s.value} className="mt-0.5" />
              <div className="ui-text-sm">
                <div className="font-medium">{s.label}</div>
                <div className="ui-text-xs text-muted-foreground">{s.desc}</div>
              </div>
            </label>
          ))}
        </RadioGroup>
        <Button variant="default" size="sm" className="w-full" onClick={() => void handleMerge()}>
          {active.confirm}
        </Button>
      </PopoverContent>
    </Popover>
  )
}
