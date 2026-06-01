import { useRef, useState } from 'react'
import { ListBullets, X } from '@phosphor-icons/react'
import { ResizablePanelGroup, ResizablePanel, ResizableHandle } from '@/components/ui/resizable'
import { Button } from '@/components/ui/button'
import { useResizeObserver } from '@/features/panes/hooks/use-resize-observer'
import { DiffFileTree, diffFileAnchorId } from './diff-file-tree'
import { BranchReviewDiffViewer } from './branch-review-diff-viewer'
import type { MultiFileDiff } from '@/features/git/types/git-diff-types'
import type { ReviewThread } from '@/features/branch-review/types/review-types'

/** Below this pane width the file tree collapses behind a toggle. */
const NARROW_BREAKPOINT = 720

interface BranchReviewDiffPanelProps {
  multiDiff: MultiFileDiff
  threads: ReviewThread[]
  onAddThread: (filePath: string, lineNumber: number) => void
  onReply: (threadId: string, body: string) => void
  onResolve: (threadId: string) => void
}

export function BranchReviewDiffPanel(props: BranchReviewDiffPanelProps) {
  const { multiDiff } = props
  const containerRef = useRef<HTMLDivElement>(null)
  const { width } = useResizeObserver(containerRef)
  const isNarrow = width > 0 && width < NARROW_BREAKPOINT
  const [overlayOpen, setOverlayOpen] = useState(false)

  function scrollToFile(filePath: string) {
    document.getElementById(diffFileAnchorId(filePath))?.scrollIntoView({ behavior: 'smooth', block: 'start' })
    if (isNarrow) setOverlayOpen(false)
  }

  const viewer = <BranchReviewDiffViewer {...props} />

  return (
    <div ref={containerRef} className="relative h-full">
      {isNarrow ? (
        <>
          <Button
            variant="outline"
            size="icon-sm"
            onClick={() => setOverlayOpen(o => !o)}
            tooltip="Changed files"
            aria-label="Toggle changed files"
            className="absolute left-2 top-2 z-30 shadow-sm"
          >
            <ListBullets size={15} />
          </Button>

          <div className="h-full">{viewer}</div>

          {overlayOpen && (
            <>
              <button
                aria-label="Close file list"
                className="absolute inset-0 z-20 cursor-default bg-black/30"
                onClick={() => setOverlayOpen(false)}
              />
              <div className="absolute inset-y-0 left-0 z-30 flex w-64 flex-col border-r border-border/50 bg-background shadow-xl">
                <div className="flex items-center justify-between px-3 py-2">
                  <span className="text-xs font-medium text-muted-foreground">
                    {multiDiff.totalFiles} changed file{multiDiff.totalFiles !== 1 ? 's' : ''}
                  </span>
                  <Button variant="ghost" size="icon-xs" onClick={() => setOverlayOpen(false)} aria-label="Close">
                    <X size={13} />
                  </Button>
                </div>
                <div className="min-h-0 flex-1">
                  <DiffFileTree files={multiDiff.files} onSelectFile={scrollToFile} />
                </div>
              </div>
            </>
          )}
        </>
      ) : (
        <ResizablePanelGroup orientation="horizontal" className="h-full">
          <ResizablePanel defaultSize="24%" minSize="14%" maxSize="45%" className="overflow-hidden border-r border-border/50">
            <DiffFileTree files={multiDiff.files} onSelectFile={scrollToFile} />
          </ResizablePanel>
          <ResizableHandle withHandle />
          <ResizablePanel defaultSize="76%" className="overflow-hidden">
            {viewer}
          </ResizablePanel>
        </ResizablePanelGroup>
      )}
    </div>
  )
}
