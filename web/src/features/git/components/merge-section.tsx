import { GitMerge, Warning } from '@phosphor-icons/react'
import { Button } from '@/components/ui/button'
import { toast } from '@/features/window/stores/toast-store'
import { useWorkspaceStoreById } from '@/features/workspace/stores/hooks/use-workspace-store-by-id'
import { getOrCreateWorkspaceStore } from '@/features/workspace/stores/workspace-store-registry'
import type { MergeStrategy } from '@/features/workspace/stores/slices/branch-review-slice'
import { setMergeStrategy as patchMergeStrategy, mergeIntoParent } from '../api/review-api'
import { resolveMergeState } from '../lib/merge-section-state'

interface MergeSectionProps {
  wsId: string
  parentBranch: string
  canMergeLocally: boolean
  hasUncommitted: boolean
  status: string
}

const STRATEGIES: { value: MergeStrategy; label: string }[] = [
  { value: 'merge', label: 'Merge' },
  { value: 'squash', label: 'Squash' },
  { value: 'rebase', label: 'Rebase' },
]

function MergeStrategySelector({ wsId }: { wsId: string }) {
  const mergeStrategy = useWorkspaceStoreById(wsId, (s) => s.branchReview.mergeStrategy)

  const handleSelect = async (next: MergeStrategy) => {
    if (next === mergeStrategy) return
    const previous = mergeStrategy
    getOrCreateWorkspaceStore(wsId).getState().setBranchReviewMergeStrategy(next)
    try {
      await patchMergeStrategy(wsId, next)
    } catch {
      getOrCreateWorkspaceStore(wsId).getState().setBranchReviewMergeStrategy(previous)
      toast.error('Failed to update merge strategy')
    }
  }

  return (
    <div className="flex flex-col gap-1.5">
      <span className="ui-text-xs flex items-center gap-1.5 font-medium text-muted-foreground">
        <GitMerge className="size-3.5" />
        Merge strategy
      </span>
      <div className="flex gap-1">
        {STRATEGIES.map((strategy) => (
          <Button
            key={strategy.value}
            size="sm"
            variant={strategy.value === mergeStrategy ? 'default' : 'outline'}
            onClick={() => void handleSelect(strategy.value)}
          >
            {strategy.label}
          </Button>
        ))}
      </div>
    </div>
  )
}

export function MergeSection({
  wsId,
  parentBranch,
  canMergeLocally,
  hasUncommitted,
  status,
}: MergeSectionProps) {
  const mergeStrategy = useWorkspaceStoreById(wsId, (s) => s.branchReview.mergeStrategy)
  const mergeState = resolveMergeState({ canMergeLocally, hasUncommitted, status })

  const handleMerge = async () => {
    try {
      await mergeIntoParent(wsId, mergeStrategy)
      toast.info('Merging…')
    } catch {
      toast.error('Merge failed — check the logs for details')
    }
  }

  const handleConflict = () => {
    // NOTE: No dedicated conflict-resolution entry point was found in the
    // codebase at the time of implementation. The git-changes-panel and
    // git-status-panel surface changed files but do not expose a conflict
    // resolver route/dialog. This is a toast placeholder until that surface
    // is built — see Task 4 report for details.
    toast.warning(
      'Open the conflicting files and resolve conflicts, then commit.',
      'Merge conflicts detected',
    )
  }

  return (
    <div className="flex flex-col gap-4">
      <MergeStrategySelector wsId={wsId} />

      <div className="flex flex-col gap-2">
        {mergeState.kind === 'eligible' && (
          <p className="ui-text-xs text-muted-foreground">
            <span className="font-mono">{parentBranch}</span> is local &amp; unprotected
          </p>
        )}

        {mergeState.kind === 'conflict' && (
          <div className="flex items-center gap-1.5 rounded-md border border-destructive/32 bg-destructive/8 px-2.5 py-1.5">
            <Warning className="size-3.5 shrink-0 text-destructive" />
            <span className="ui-text-xs text-destructive">
              Merge conflicts must be resolved before merging.
            </span>
          </div>
        )}

        {mergeState.kind === 'eligible' && (
          <Button size="sm" variant="default" onClick={() => void handleMerge()}>
            Merge into {parentBranch}
          </Button>
        )}

        {mergeState.kind === 'protected' && (
          <Button size="sm" variant="outline" disabled>
            {parentBranch} is protected — open a pull request
          </Button>
        )}

        {mergeState.kind === 'conflict' && (
          <Button size="sm" variant="destructive" onClick={handleConflict}>
            Resolve conflicts
          </Button>
        )}
      </div>
    </div>
  )
}
