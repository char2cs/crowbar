import { useState } from 'react'
import { GitMerge } from '@phosphor-icons/react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { useWorkspaceStoreById } from '@/features/workspace/stores/hooks/use-workspace-store-by-id'
import { useSidebarStore } from '@/lib/store/sidebar'
import { useBranchMerge } from '../hooks/use-branch-merge'
import { resolveBranchAction } from '../lib/branch-action'
import { MERGE_STRATEGIES } from '../lib/merge-strategies'
import type { MergeStrategy } from '@/features/workspace/stores/slices/branch-review-slice'

function plural(n: number, noun: string): string {
  return `${n} ${noun}${n === 1 ? '' : 's'}`
}

/** Why Merge is unavailable, or null when it is allowed. Open threads block it too. */
function blockedReason(
  kind: ReturnType<typeof resolveBranchAction>['kind'],
  parentBranch: string | undefined,
  uncommitted: number,
  openThreads: number,
): string | null {
  if (kind === 'commit') return `${plural(uncommitted, 'uncommitted change')} — commit them first.`
  if (kind === 'resolve') return `Conflicts with ${parentBranch} — resolve them first.`
  if (kind === 'pull-request') return `${parentBranch} is protected — open a pull request instead.`
  if (kind === 'sync-only') return 'This branch has no parent to merge into.'
  if (openThreads > 0)
    return `${plural(openThreads, 'open thread')} must be resolved before merging.`
  return null
}

export function ReviewMergeBox({ wsId }: { wsId: string }) {
  const ws = useSidebarStore((s) => {
    for (const repo of s.repos) {
      const found = repo.workspaces.find((w) => w.id === wsId)
      if (found) return found
    }
    return null
  })
  const gitStatus = useWorkspaceStoreById(wsId, (s) => s.gitStatus)
  const openThreads = useWorkspaceStoreById(
    wsId,
    (s) => s.branchReview.threads.filter((t) => !t.isResolved).length,
  )
  const { strategy, selectStrategy, strategyError, merge, merging, mergeError } =
    useBranchMerge(wsId)
  // Default ON, as in the sidebar popover: merging a child usually means it is done.
  const [deleteAfterMerge, setDeleteAfterMerge] = useState(true)

  const parentBranch = ws?.parentBranch
  const uncommitted = gitStatus?.files.length ?? 0
  const action = resolveBranchAction({
    hasUncommitted: uncommitted > 0,
    hasParent: Boolean(parentBranch),
    canMergeLocally: ws?.canMergeLocally ?? false,
    status: ws?.status ?? 'new',
  })
  const merged = ws?.status === 'pr-merged'
  const reason = merged ? null : blockedReason(action.kind, parentBranch, uncommitted, openThreads)
  const active = MERGE_STRATEGIES.find((s) => s.value === strategy) ?? MERGE_STRATEGIES[0]

  return (
    <Card aria-label="Merge" role="region" className="mx-1.5 my-1 min-w-0 gap-0 rounded-xl">
      <CardHeader className="p-3">
        <CardTitle className="ui-text-sm flex min-w-0 items-center gap-2">
          <GitMerge className="size-4 shrink-0 text-muted-foreground" />
          <span className="min-w-0 truncate">
            {merged ? `Merged into ${parentBranch}` : `Merge into ${parentBranch ?? 'parent'}`}
          </span>
        </CardTitle>
      </CardHeader>

      {merged ? null : (
        <CardContent className="flex min-w-0 flex-col gap-2 p-3">
          <div className="flex min-w-0 flex-col gap-2">
            <Select
              value={strategy}
              onValueChange={(value) => void selectStrategy(value as MergeStrategy)}
            >
              <SelectTrigger
                size="sm"
                aria-label="Merge strategy"
                className="w-full min-w-0 whitespace-nowrap"
              >
                <SelectValue>{active.label}</SelectValue>
              </SelectTrigger>
              <SelectContent>
                {MERGE_STRATEGIES.map((s) => (
                  <SelectItem key={s.value} value={s.value}>
                    {s.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Button
              size="sm"
              className="w-full whitespace-nowrap"
              disabled={reason !== null || merging}
              onClick={() => void merge(deleteAfterMerge)}
            >
              {merging ? 'Merging…' : active.confirm}
            </Button>
          </div>

          <label className="flex cursor-pointer items-start gap-2">
            <Checkbox checked={deleteAfterMerge} onChange={setDeleteAfterMerge} />
            <span className="ui-text-xs min-w-0">Delete workspace after merging</span>
          </label>

          {reason && <p className="ui-text-xs min-w-0 text-muted-foreground">{reason}</p>}
          {strategyError && <p className="ui-text-xs text-destructive">{strategyError}</p>}
          {mergeError && <p className="ui-text-xs text-destructive">{mergeError}</p>}
        </CardContent>
      )}
    </Card>
  )
}
