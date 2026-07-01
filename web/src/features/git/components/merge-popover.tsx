import type { ReactElement } from 'react'
import { useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { Popover, PopoverTrigger, PopoverContent, PopoverTitle } from '@/components/ui/popover'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { RadioGroup, Radio } from '@/components/ui/radio-group'
import { useWorkspaceStoreById } from '@/features/workspace/stores/hooks/use-workspace-store-by-id'
import { getOrCreateWorkspaceStore } from '@/features/workspace/stores/workspace-store-registry'
import { useSidebarStore, getPostDeleteNavigationTarget } from '@/lib/store/sidebar'
import { setMergeStrategy as patchMergeStrategy, mergeIntoParent } from '../api/review-api'
import type { MergeStrategy } from '@/features/workspace/stores/slices/branch-review-slice'

const STRATEGIES: { value: MergeStrategy; label: string; confirm: string; desc: string }[] = [
  {
    value: 'merge',
    label: 'Create a merge commit',
    confirm: 'Create merge commit',
    desc: 'All commits, plus a merge commit',
  },
  {
    value: 'squash',
    label: 'Squash and merge',
    confirm: 'Squash & merge',
    desc: 'One combined commit',
  },
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
  // Default ON: merging a child into its parent usually means you're done with it.
  const [deleteAfterMerge, setDeleteAfterMerge] = useState(true)
  const [mergeError, setMergeError] = useState<string | null>(null)
  const [strategyError, setStrategyError] = useState<string | null>(null)
  const [merging, setMerging] = useState(false)
  const navigate = useNavigate()
  const strategy = useWorkspaceStoreById(wsId, (s) => s.branchReview.mergeStrategy)
  const active = STRATEGIES.find((s) => s.value === strategy) ?? STRATEGIES[0]

  // Persist the chosen strategy so it becomes the default next time (optimistic).
  const selectStrategy = async (next: MergeStrategy) => {
    if (next === strategy) return
    const previous = strategy
    setStrategyError(null)
    getOrCreateWorkspaceStore(wsId).getState().setBranchReviewMergeStrategy(next)
    try {
      await patchMergeStrategy(wsId, next)
    } catch {
      getOrCreateWorkspaceStore(wsId).getState().setBranchReviewMergeStrategy(previous)
      setStrategyError('Failed to save strategy — try again')
    }
  }

  const handleMerge = async () => {
    setMergeError(null)
    setMerging(true)
    let redirect: { projectId: string; repoId: string; wsId: string } | null = null
    if (deleteAfterMerge) {
      const repos = useSidebarStore.getState().repos
      const targetWsId = getPostDeleteNavigationTarget(repos, wsId)
      const repo = targetWsId
        ? repos.find((r) => r.workspaces.some((w) => w.id === targetWsId))
        : undefined
      if (targetWsId && repo) {
        redirect = { projectId: repo.projectId ?? '', repoId: repo.id, wsId: targetWsId }
      }
    }
    try {
      await mergeIntoParent(wsId, strategy, deleteAfterMerge)
      setOpen(false)
      if (redirect) void navigate({ to: '/ide/$projectId/$repoId/$wsId', params: redirect })
    } catch {
      setMergeError('Merge failed — check the logs for details')
    } finally {
      setMerging(false)
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
            <label
              key={s.value}
              className="flex cursor-pointer items-start gap-2 rounded-md py-0.5"
            >
              <Radio value={s.value} className="mt-0.5" />
              <div className="ui-text-sm">
                <div className="font-medium">{s.label}</div>
                <div className="ui-text-xs text-muted-foreground">{s.desc}</div>
              </div>
            </label>
          ))}
        </RadioGroup>
        {strategyError && <p className="ui-text-xs text-destructive mb-2">{strategyError}</p>}
        <label className="mb-3 flex cursor-pointer items-center gap-2">
          <Checkbox checked={deleteAfterMerge} onChange={setDeleteAfterMerge} />
          <span className="ui-text-sm">Delete this workspace after merging</span>
        </label>
        <Button
          variant="default"
          size="sm"
          className="w-full"
          disabled={merging}
          onClick={() => void handleMerge()}
        >
          {merging ? 'Merging…' : active.confirm}
        </Button>
        {mergeError && <p className="ui-text-xs text-destructive mt-2">{mergeError}</p>}
      </PopoverContent>
    </Popover>
  )
}
