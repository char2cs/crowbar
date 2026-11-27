import { useCallback, useMemo, useRef } from 'react'
import type {
  CodeViewItem,
  CodeViewOptions,
  FileDiffLoadedFiles,
  FileDiffMetadata,
} from '@pierre/diffs'
import { WorkerPoolContextProvider } from '@pierre/diffs/react'
import HighlightWorker from '@pierre/diffs/worker/worker.js?worker'
import { getReviewFileSides } from '@/features/git/api/review-window-api'
import type { FileOutline } from '@/features/git/api/review-window-api'
import { MAX_MATERIALIZED_LINES } from '@/features/git/lib/patch-window'
import {
  buildPlaceholderFileDiff,
  partitionReviewFiles,
  signatureOf,
} from '@/features/git/lib/review-placeholder'
import { isFileViewed, useReviewViewed } from '@/features/git/lib/review-viewed'
import { usePreservedScroll } from '@/features/editor/hooks/use-preserved-scroll'
import type { GitDiff } from '@/features/git/types/git-types'
import type { ReviewThread } from '@/features/workspace/stores/slices/branch-review-slice'
import { cn } from '@/utils/cn'
import { ReviewBinaryFiles } from './review-binary-files'
import { FileThreadCount, PatchStateNotice, ViewedToggle } from './review-patch-header'
import { useReviewAnnotations } from './use-review-annotations'
import type { ReviewAnnotation } from './use-review-annotations'
import { useReviewPatchWindow } from './use-review-patch-window'
import { DiffCodeView } from './diff-code-view'

export const REVIEW_TOKENIZE_MAX_LINE_LENGTH = 2_000

export const REVIEW_TOKENIZE_MAX_LENGTH = MAX_MATERIALIZED_LINES

const HIGHLIGHT_POOL_SIZE = 2

const FILE_CARD_CSS = `[data-diffs-header] {
  background-color: color-mix(in srgb, var(--background) 94%, var(--foreground));
  box-shadow: inset 0 -1px 0 var(--border);
}`

const FILE_CARD_GAP = 12

const EXPANSION_LINE_COUNT = 20

export interface ReviewCodeViewHandle {
  revealLine: (path: string, lineNumber: number, side: 'old' | 'new') => void
  revealFile: (path: string) => void
}

export interface ReviewCodeViewProps {
  wsId: string
  files: readonly GitDiff[]
  outline: readonly FileOutline[]
  commit?: string
  isActivePane?: boolean
  diffStyle?: 'split' | 'unified'
  surfaceRef?: React.Ref<ReviewCodeViewHandle>
  className?: string
}

let workerPoolUsable: boolean | undefined
function canUseWorkerPool(): boolean {
  if (workerPoolUsable !== undefined) return workerPoolUsable
  if (typeof Worker === 'undefined') {
    workerPoolUsable = false
    return workerPoolUsable
  }
  try {
    createHighlightWorker().terminate()
    workerPoolUsable = true
  } catch {
    workerPoolUsable = false
  }
  return workerPoolUsable
}

export function ReviewCodeView(props: ReviewCodeViewProps) {
  if (!canUseWorkerPool()) return <ReviewCodeViewSurface {...props} />
  return (
    <WorkerPoolContextProvider
      poolOptions={{ workerFactory: createHighlightWorker, poolSize: HIGHLIGHT_POOL_SIZE }}
      highlighterOptions={{ tokenizeMaxLineLength: REVIEW_TOKENIZE_MAX_LINE_LENGTH }}
    >
      <ReviewCodeViewSurface {...props} />
    </WorkerPoolContextProvider>
  )
}

function createHighlightWorker(): Worker {
  return new HighlightWorker()
}

function ReviewCodeViewSurface({
  wsId,
  commit,
  files,
  outline,
  isActivePane = true,
  diffStyle = 'split',
  className,
  surfaceRef,
}: ReviewCodeViewProps) {
  const entries = useMemo(() => partitionReviewFiles(files, outline), [files, outline])
  const binaries = useMemo(() => entries.filter((e) => e.kind !== 'diff'), [entries])

  const { viewed, setFilesViewed } = useReviewViewed()
  const viewedPaths = useMemo(() => {
    const set = new Set<string>()
    for (const entry of entries) {
      if (entry.kind === 'diff' && isFileViewed(viewed, entry.file)) set.add(entry.path)
    }
    return set
  }, [entries, viewed])

  const placeholders = useMemo(() => {
    const map = new Map<string, FileDiffMetadata>()
    for (const entry of entries) {
      if (entry.kind !== 'diff') continue
      map.set(entry.path, buildPlaceholderFileDiff(entry.file, entry.outline))
    }
    return map
  }, [entries])

  const filesByPath = useMemo(() => new Map(entries.map((e) => [e.path, e.file])), [entries])
  const toggleViewed = useCallback(
    (path: string, next: boolean) => {
      const file = filesByPath.get(path)
      if (file != null) setFilesViewed([file], next)
    },
    [filesByPath, setFilesViewed],
  )

  const annotations = useReviewAnnotations({ wsId })
  const { annotationsByPath, threadCounts, threadsFor } = annotations

  const items = useMemo<CodeViewItem<ReviewThread>[]>(
    () =>
      [...placeholders].map(([path, fileDiff]) => ({
        id: path,
        type: 'diff',
        fileDiff,
        annotations: annotationsByPath.get(path),
        collapsed: viewedPaths.has(path),
        version: 0,
      })),
    [annotationsByPath, placeholders, viewedPaths],
  )
  const paths = useMemo(() => items.map((item) => item.id), [items])
  const signature = useMemo(() => signatureOf(paths), [paths])
  const lineCounts = useMemo(() => {
    const counts: Record<string, number> = {}
    for (const [path, fileDiff] of placeholders) counts[path] = fileDiff.unifiedLineCount
    return counts
  }, [placeholders])

  const scrollKey = `${wsId}\u0000${commit ?? ''}`
  const scrollerRef = useRef<HTMLDivElement | null>(null)
  const markScrollerRef = useCallback((node: HTMLDivElement | null) => {
    scrollerRef.current = node
    if (node != null) node.dataset.reviewCodeViewScroller = ''
  }, [])

  const { handleRef, patchStates, runWindow, expandTruncated, retryPatch, revealThread } =
    useReviewPatchWindow({
      wsId,
      commit,
      isActivePane,
      placeholders,
      items,
      paths,
      lineCounts,
      viewedPaths,
      signature,
      annotationsByPath,
      surfaceRef,
    })

  const revealFirstThread = useCallback(
    (path: string) => {
      const [first] = threadsFor(path)
      if (first != null) void revealThread(first)
    },
    [revealThread, threadsFor],
  )

  usePreservedScroll(scrollerRef, scrollKey, items.length > 0)

  const loadDiffFiles = useCallback(
    async (fileDiff: FileDiffMetadata): Promise<FileDiffLoadedFiles> => {
      const sides = await getReviewFileSides({ wsId, commit }, fileDiff.name, fileDiff.prevName)
      if (sides.old == null || sides.new == null) {
        throw new Error(`cannot reveal unchanged lines of ${fileDiff.name}: a side is missing`)
      }
      return {
        oldFile: { name: fileDiff.prevName ?? fileDiff.name, contents: sides.old },
        newFile: { name: fileDiff.name, contents: sides.new },
      }
    },
    [wsId, commit],
  )

  const options = useMemo<CodeViewOptions<ReviewThread, undefined>>(
    () => ({
      stickyHeaders: true,
      tokenizeMaxLineLength: REVIEW_TOKENIZE_MAX_LINE_LENGTH,
      tokenizeMaxLength: REVIEW_TOKENIZE_MAX_LENGTH,
      enableLineSelection: true,
      enableGutterUtility: true,
      diffStyle,
      unsafeCSS: FILE_CARD_CSS,
      layout: { paddingTop: FILE_CARD_GAP, paddingBottom: FILE_CARD_GAP, gap: FILE_CARD_GAP },
      loadDiffFiles,
      expansionLineCount: EXPANSION_LINE_COUNT,
    }),
    [diffStyle, loadDiffFiles],
  )

  return (
    <div className={cn('flex h-full min-h-0 flex-col', className)}>
      {binaries.length > 0 ? <ReviewBinaryFiles entries={binaries} /> : null}
      <DiffCodeView<ReviewThread>
        viewKey={signature}
        ref={handleRef}
        containerRef={markScrollerRef}
        initialItems={items}
        options={options}
        onScroll={runWindow}
        onSelectedLinesChange={annotations.onSelectedLinesChange}
        className="min-h-0 flex-1 overflow-y-auto px-3"
        // react-doctor-disable-next-line no-render-prop-children -- accepted: the render slots are the @pierre/diffs CodeView API this wrapper forwards unchanged.
        renderAnnotation={(annotation) =>
          annotations.renderAnnotation(annotation as ReviewAnnotation)
        }
        renderGutterUtility={annotations.renderGutterUtility}
        renderHeaderMetadata={(item) => {
          const count = threadCounts[item.id] ?? 0
          const state = patchStates[item.id]
          return (
            <span className="flex items-center gap-2">
              <ViewedToggle
                path={item.id}
                viewed={viewedPaths.has(item.id)}
                onToggle={toggleViewed}
              />
              <FileThreadCount path={item.id} count={count} onReveal={revealFirstThread} />
              <PatchStateNotice
                path={item.id}
                state={state}
                onExpand={expandTruncated}
                onRetry={retryPatch}
              />
            </span>
          )
        }}
      />
    </div>
  )
}
