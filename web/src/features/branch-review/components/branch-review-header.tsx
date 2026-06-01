import { GitBranch } from '@phosphor-icons/react'
import { Badge } from '@/components/ui/badge'
import { MergeButton } from './merge-button'
import type { MergeStrategy } from '@/features/branch-review/types/review-types'

interface BranchReviewHeaderProps {
  branchName: string
  parentBranch: string
  isLocked: boolean
  hasConflicts: boolean
  aheadCount: number
  additions: number
  deletions: number
  mergeStrategy: MergeStrategy
  onMerge: () => void
  onStrategyChange: (strategy: MergeStrategy) => void
}

export function BranchReviewHeader({
  branchName, parentBranch, isLocked, hasConflicts,
  aheadCount, additions, deletions, mergeStrategy, onMerge, onStrategyChange,
}: BranchReviewHeaderProps) {
  const stats = [
    aheadCount > 0 && `${aheadCount} commit${aheadCount !== 1 ? 's' : ''} ahead`,
    additions > 0 && `${additions.toLocaleString()} additions`,
    deletions > 0 && `${deletions.toLocaleString()} deletions`,
  ].filter(Boolean).join(' · ')

  return (
    <div className="flex items-center justify-between px-4 pt-4 pb-3">
      <div>
        <div className="flex items-center gap-2 text-sm font-semibold text-foreground">
          <GitBranch size={14} className="text-muted-foreground" />
          {branchName}
          <Badge variant="outline" className="text-xs font-normal text-muted-foreground">
            → {parentBranch}
          </Badge>
        </div>
        {stats && <p className="mt-0.5 text-xs text-muted-foreground">{stats}</p>}
      </div>
      <MergeButton
        strategy={mergeStrategy}
        isLocked={isLocked}
        hasConflicts={hasConflicts}
        onMerge={onMerge}
        onStrategyChange={onStrategyChange}
      />
    </div>
  )
}
