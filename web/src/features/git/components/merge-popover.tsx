import type { ReactElement } from 'react'
import { useEffect, useRef, useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { Popover, PopoverTrigger, PopoverContent, PopoverTitle } from '@/components/ui/popover'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { RadioGroup, Radio } from '@/components/ui/radio-group'
import { useWorkspaceStoreById } from '@/features/workspace/stores/hooks/use-workspace-store-by-id'
import {
  getActiveWorkspaceId,
  getWorkspaceStore,
  subscribeActiveWorkspaceId,
} from '@/features/workspace/stores/workspace-store-registry'
import { isEntityPresent, subscribeEntityPresence } from '@/lib/store/entity-presence'
import { selectEntityBusy, useOperationsStore } from '@/lib/store/operations'
import { useSidebarStore } from '@/lib/store/sidebar'
import { getPostDeleteNavigationTarget } from '@/lib/store/repo-tree'
import { runOperation, whenSettled } from '@/lib/store/run-operation'
import { setMergeStrategy as patchMergeStrategy, mergeIntoParent } from '../api/review-api'
import type { MergeStrategy } from '@/features/workspace/stores/slices/branch-review-slice'

/** Calls `go` once `wsId` has left the sidebar, whichever frame lands first, while
 *  the user still shows it; leaving it (or the returned stop) ends the wait. A
 *  merge that kept the child never sees it leave, so it never navigates. */
function whenChildLeaves(wsId: string, go: () => void): () => void {
  const stops: (() => void)[] = []
  const stop = () => stops.splice(0).forEach((s) => s())
  const check = () => {
    if (getActiveWorkspaceId() !== wsId) return stop()
    if (isEntityPresent({ kind: 'workspace', id: wsId })) return
    stop()
    go()
  }
  stops.push(subscribeEntityPresence(check), subscribeActiveWorkspaceId(check))
  check()
  return stop
}

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
  const [strategyError, setStrategyError] = useState<string | null>(null)
  const navigate = useNavigate()
  // One hierarchy verb at a time per workspace; the daemon refuses a second too.
  const busy = useOperationsStore((s) => selectEntityBusy(s, wsId))
  const strategy = useWorkspaceStoreById(wsId, (s) => s.branchReview.mergeStrategy)
  const active = STRATEGIES.find((s) => s.value === strategy) ?? STRATEGIES[0]

  // Persist the chosen strategy so it becomes the default next time (optimistic).
  const selectStrategy = async (next: MergeStrategy) => {
    if (next === strategy) return
    const previous = strategy
    setStrategyError(null)
    getWorkspaceStore(wsId)?.getState().setBranchReviewMergeStrategy(next)
    try {
      await patchMergeStrategy(wsId, next)
    } catch {
      getWorkspaceStore(wsId)?.getState().setBranchReviewMergeStrategy(previous)
      setStrategyError('Failed to save strategy — try again')
    }
  }

  // The merge runs as an operation: its failure is the operations watcher's to
  // show, and the redirect waits for the merge (and the fold) to land.
  const stopWaiting = useRef<(() => void) | null>(null)
  useEffect(() => () => stopWaiting.current?.(), [])

  const handleMerge = () => {
    if (busy) return
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
    const op = runOperation(
      {
        kind: 'workspace.merge_into_parent',
        subject: { kind: 'workspace', id: wsId },
        projectId: redirect?.projectId ?? '',
        label: `Merge into ${parentBranch}`,
      },
      (opId) => mergeIntoParent(wsId, strategy, deleteAfterMerge, opId),
    )
    setOpen(false)
    void whenSettled(op.id).then(
      () => {
        const target = redirect
        if (!target) return
        stopWaiting.current?.()
        stopWaiting.current = whenChildLeaves(wsId, () => {
          void navigate({ to: '/ide/$projectId/$repoId/$wsId', params: target })
        })
      },
      () => undefined,
    )
  }

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger render={trigger} disabled={busy} />
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
          disabled={busy}
          onClick={handleMerge}
        >
          {active.confirm}
        </Button>
      </PopoverContent>
    </Popover>
  )
}
