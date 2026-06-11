import { GitMerge } from '@phosphor-icons/react'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import { ScrollArea } from '@/components/ui/scroll-area'
import { cn } from '@/utils/cn'
import { toast } from '@/features/window/stores/toast-store'
import {
  useWorkspaceStore,
  useWorkspaceStoreContext,
} from '@/features/workspace/stores/workspace-context'
import type { MergeStrategy } from '@/features/workspace/stores/slices/branch-review-slice'
import { setMergeStrategy as patchMergeStrategy } from '../api/review-api'

interface ReviewAboutTabProps {
  wsId: string
  isLoading: boolean
}

const STRATEGIES: { value: MergeStrategy; label: string }[] = [
  { value: 'merge', label: 'Merge' },
  { value: 'squash', label: 'Squash' },
  { value: 'rebase', label: 'Rebase' },
]

function MergeStrategySelector({ wsId }: { wsId: string }) {
  const mergeStrategy = useWorkspaceStoreContext((s) => s.branchReview.mergeStrategy)
  const store = useWorkspaceStore()

  const handleSelect = async (next: MergeStrategy) => {
    if (next === mergeStrategy) return
    const previous = mergeStrategy
    store.getState().setBranchReviewMergeStrategy(next)
    try {
      await patchMergeStrategy(wsId, next)
    } catch {
      store.getState().setBranchReviewMergeStrategy(previous)
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

export function ReviewAboutTab({ wsId, isLoading }: ReviewAboutTabProps) {
  const description = useWorkspaceStoreContext((s) => s.branchReview.description)
  const store = useWorkspaceStore()

  return (
    <ScrollArea className="h-full">
      <div className="flex flex-col gap-4 p-4">
        <MergeStrategySelector wsId={wsId} />

        <div className="flex flex-col gap-1.5">
          <span className="ui-text-xs font-medium text-muted-foreground">Description</span>
          {isLoading ? (
            <div className="h-24 animate-pulse rounded-md bg-muted" />
          ) : (
            <Textarea
              value={description}
              onChange={(e) => store.getState().setBranchReviewDescription(e.target.value)}
              placeholder="Describe what this branch changes…"
              rows={8}
              className={cn('ui-text-sm', description.length === 0 && 'text-muted-foreground')}
            />
          )}
          {!isLoading && description.length === 0 && (
            <span className="ui-text-xs text-muted-foreground">
              No description yet — summarize the intent of this branch.
            </span>
          )}
        </div>
      </div>
    </ScrollArea>
  )
}
