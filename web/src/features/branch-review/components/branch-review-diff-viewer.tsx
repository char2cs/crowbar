import { DiffFileSection } from './diff-file-section'
import type { MultiFileDiff } from '@/features/git/types/git-diff-types'
import type { ReviewThread } from '@/features/branch-review/types/review-types'

interface BranchReviewDiffViewerProps {
  multiDiff: MultiFileDiff
  threads: ReviewThread[]
  onAddThread: (filePath: string, lineNumber: number) => void
  onReply: (threadId: string, body: string) => void
  onResolve: (threadId: string) => void
}

export function BranchReviewDiffViewer({
  multiDiff,
  threads,
  onAddThread,
  onReply,
  onResolve,
}: BranchReviewDiffViewerProps) {
  return (
    <div className="h-full overflow-y-auto">
      {multiDiff.files.map((file, index) => {
        const fileThreads = threads.filter(t => t.filePath === file.file_path)
        return (
          <DiffFileSection
            key={`${file.file_path}:${index}`}
            diff={file}
            threads={fileThreads}
            onAddThread={onAddThread}
            onReply={onReply}
            onResolve={onResolve}
          />
        )
      })}
    </div>
  )
}
