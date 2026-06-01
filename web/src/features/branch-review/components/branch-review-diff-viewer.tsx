import { DiffFileSection } from './diff-file-section'
import { diffFileAnchorId } from './diff-file-tree'
import type { MultiFileDiff } from '@/features/git/types/git-diff-types'
import type { ReviewThread } from '@/features/branch-review/types/review-types'

interface BranchReviewDiffViewerProps {
  multiDiff: MultiFileDiff
  threads: ReviewThread[]
  onAddThread: (filePath: string, lineNumber: number, side: 'left' | 'right') => void
  onReply: (threadId: string, body: string) => void
  onResolve: (threadId: string) => void
  onDelete: (threadId: string) => void
}

export function BranchReviewDiffViewer({
  multiDiff,
  threads,
  onAddThread,
  onReply,
  onResolve,
  onDelete,
}: BranchReviewDiffViewerProps) {
  return (
    <div className="h-full overflow-y-auto">
      {multiDiff.files.map((file, index) => {
        const fileThreads = threads.filter(t => t.filePath === file.file_path)
        return (
          <div key={`${file.file_path}:${index}`} id={diffFileAnchorId(file.file_path)} className="scroll-mt-2">
            <DiffFileSection
              diff={file}
              threads={fileThreads}
              onAddThread={onAddThread}
              onReply={onReply}
              onResolve={onResolve}
              onDelete={onDelete}
            />
          </div>
        )
      })}
    </div>
  )
}
