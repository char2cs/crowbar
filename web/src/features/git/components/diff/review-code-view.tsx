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

// The windowed Branch Review: files are laid out from the outline's hunk geometry and only patches
// near the viewport are held, so memory is bounded by the window. Placeholders get heights right
// first; materialising is imperative (updateItem); binaries never reach the highlighter.

// ── Tunables ────────────────────────────────────────────────────────

/** Longest line Shiki tokenises: a minified line (650k chars) blocks the drawing thread for
 *  seconds. 2,000 is far past hand-written code and far short of minified. */
export const REVIEW_TOKENIZE_MAX_LINE_LENGTH = 2_000

/** Rendered lines past which a diff is plain text — for files huge rather than wide. Set at the
 *  planner's window budget, so a file filling it alone still renders, unhighlighted. */
export const REVIEW_TOKENIZE_MAX_LENGTH = MAX_MATERIALIZED_LINES

/** Each worker holds its own highlighter (tens of MB), so not the library's 8: two keep
 *  highlighting off the scrolling thread. */
const HIGHLIGHT_POOL_SIZE = 2

/** Raised header tone, opaque so the sticky header never shows code through it.
 *  Inset shadow, not a border: the virtualizer's header height must not change. */
const FILE_CARD_CSS = `[data-diffs-header] {
  background-color: color-mix(in srgb, var(--background) 94%, var(--foreground));
  box-shadow: inset 0 -1px 0 var(--border);
}`

/** Space between file cards; the virtualizer adds it to its own layout. */
const FILE_CARD_GAP = 12

/** Unchanged lines one click on a separator reveals, as GitHub does. */
const EXPANSION_LINE_COUNT = 20

// ── The surface ─────────────────────────────────────────────────────

/** For callers outside the surface (find-in-diff queries the daemon). Revealing is imperative: the
 *  target may be a placeholder, so only the surface can materialise, flush, then scroll. */
export interface ReviewCodeViewHandle {
  revealLine: (path: string, lineNumber: number, side: 'old' | 'new') => void
  /** Bring a whole file into view (the sidebar's ask): no line to aim at, and a file's first
   *  diff line is not line 1. */
  revealFile: (path: string) => void
}

export interface ReviewCodeViewProps {
  wsId: string
  /** The branch-review summary: one row per changed file, in render order. */
  files: readonly GitDiff[]
  /** Hunk geometry for the same files. Order does not matter; it is joined by path. */
  outline: readonly FileOutline[]
  /** Scopes every read to one commit. Also in the patch cache key: @pierre/diffs treats it as
   *  item identity, and a branch/commit collision would keep the first one's body and height. */
  commit?: string
  /** False while the pane is hidden, which suspends fetching for it. */
  isActivePane?: boolean
  /** @pierre/diffs' `diffStyle`: 'split' (default) side by side, 'unified' interleaved. */
  diffStyle?: 'split' | 'unified'
  /** Navigates the surface from outside (find-in-diff resolves hits against the daemon). */
  surfaceRef?: React.Ref<ReviewCodeViewHandle>
  className?: string
}

// Probes by building a worker: `Worker` can exist yet its constructor throw (CSP, custom scheme),
// and an installed pool that can't spawn renders nothing at all. Without the provider CodeView
// highlights inline. Only a synchronous failure is caught; verify-worker-bundles.mjs gates the rest.
let workerPoolUsable: boolean | undefined
function canUseWorkerPool(): boolean {
  if (workerPoolUsable !== undefined) return workerPoolUsable
  if (typeof Worker === 'undefined') {
    workerPoolUsable = false
    return workerPoolUsable
  }
  try {
    // The pool fetches this same chunk, so the probe costs a cache hit, not a download.
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

// `?worker` makes the package's worker module the entry itself: @pierre/diffs marks it
// side-effect free, so a side-effect import may be tree-shaken into an empty, silent worker.
// verify-worker-bundles.mjs (postbuild) fails the build on an empty worker chunk.
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
    // `initialItems` is seeded once, so this only decides which threads exist at
    // MOUNT; useReviewPatchWindow republishes every later change.
    [annotationsByPath, placeholders, viewedPaths],
  )
  const paths = useMemo(() => items.map((item) => item.id), [items])
  const signature = useMemo(() => signatureOf(paths), [paths])
  const lineCounts = useMemo(() => {
    const counts: Record<string, number> = {}
    for (const [path, fileDiff] of placeholders) counts[path] = fileDiff.unifiedLineCount
    return counts
  }, [placeholders])

  // A tab switch unmounts this surface, so scroll is preserved outside it — keyed by wsId and
  // commit so a branch review and each commit tab of one workspace keep their own offset.
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

  // Restored once items exist in the DOM — before that the scroller has no
  // room to hold an offset and it would be clamped away.
  usePreservedScroll(scrollerRef, scrollKey, items.length > 0)

  // The renderer calls this only when the reader expands a separator, so a file's
  // full text is fetched on demand and never for the window at large.
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
      // Selecting a line range is how a comment is started, and the hovered "+"
      // is how a single-line one is. Both feed the same draft.
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
        // Remounting on a changed file list is deliberate: CodeView seeds
        // `initialItems` once, and a different set of files is a different
        // document, not an update to this one.
        viewKey={signature}
        ref={handleRef}
        containerRef={markScrollerRef}
        initialItems={items}
        options={options}
        onScroll={runWindow}
        onSelectedLinesChange={annotations.onSelectedLinesChange}
        className="min-h-0 flex-1 overflow-y-auto px-3"
        // Annotations and header metadata portal into light DOM, so Tailwind and CSS vars apply.
        // The cast is safe: every item here is a diff, so no side-less file annotation arrives.
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
