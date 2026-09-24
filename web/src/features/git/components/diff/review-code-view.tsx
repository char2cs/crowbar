import { useCallback, useMemo, useRef } from 'react'
import type { CodeViewItem, CodeViewOptions, FileDiffMetadata } from '@pierre/diffs'
import { CodeView, WorkerPoolContextProvider } from '@pierre/diffs/react'
import HighlightWorker from '@pierre/diffs/worker/worker.js?worker'
import type { FileOutline } from '@/features/git/api/review-window-api'
import { MAX_MATERIALIZED_LINES } from '@/features/git/lib/patch-window'
import {
  buildPlaceholderFileDiff,
  partitionReviewFiles,
  signatureOf,
} from '@/features/git/lib/review-placeholder'
import { usePreservedScroll } from '@/features/editor/hooks/use-preserved-scroll'
import type { GitDiff } from '@/features/git/types/git-types'
import type { ReviewThread } from '@/features/workspace/stores/slices/branch-review-slice'
import { cn } from '@/utils/cn'
import { ReviewBinaryFiles } from './review-binary-files'
import { FileThreadCount, PatchStateNotice } from './review-patch-header'
import { useReviewAnnotations } from './use-review-annotations'
import type { ReviewAnnotation } from './use-review-annotations'
import { useReviewPatchWindow } from './use-review-patch-window'

/**
 * The windowed Branch Review surface.
 *
 * The old stack loaded one composite `/review` payload — 158 MB and 1.4M line
 * objects on a 1M-line branch — and handed the whole thing to the renderer.
 * This one never holds more than a window: it lays every changed file out from
 * the OUTLINE's hunk geometry (numbers only, no content), and fetches a file's
 * patch text just as the reader approaches it, dropping it again once they have
 * moved on. Memory is then bounded by the window, not by the branch.
 *
 * Three consequences shape everything below:
 *
 *  1. A file's height must be right BEFORE its patch exists, or the scrollbar
 *     lies and every scroll jumps. That is what `buildPlaceholderFileDiff` is.
 *  2. Materialisation is imperative (`CodeViewHandle.updateItem`), not a React
 *     re-render — publishing 400 items through props on every scroll frame
 *     would cost more than the diff it is trying to save.
 *  3. Anything that is not a text diff must never reach the diff renderer:
 *     binaries have no patch, and feeding one to a syntax highlighter is how a
 *     review pane hangs.
 */

// ── Tunables ────────────────────────────────────────────────────────

/**
 * Longest line Shiki will tokenise, in characters.
 *
 * The 1M-line fixture contains a minified bundle whose single line is 657,780
 * characters. Tokenising that blocks whichever thread draws it for seconds, so
 * the cap is what keeps a generated file from freezing the pane. 2,000 is far
 * past any hand-written line and far short of a minified one.
 */
export const REVIEW_TOKENIZE_MAX_LINE_LENGTH = 2_000

/**
 * Rendered-line count past which a diff is drawn as plain text.
 *
 * The per-line cap alone does not save a file that is merely enormous rather
 * than merely wide. This one is deliberately at the planner's whole-window line
 * budget: a single file that would fill the entire budget by itself still
 * renders, just without highlighting.
 */
export const REVIEW_TOKENIZE_MAX_LENGTH = MAX_MATERIALIZED_LINES

/**
 * Workers in the Shiki pool.
 *
 * Each worker holds its own highlighter, so the library's default of 8 buys
 * parallelism with tens of megabytes apiece — the exact trade this phase exists
 * to refuse. Two is enough to keep highlighting off the thread that scrolls.
 */
const HIGHLIGHT_POOL_SIZE = 2

// ── The surface ─────────────────────────────────────────────────────

/**
 * Imperative surface handle.
 *
 * Find-in-diff lives outside this component (it queries the daemon, not the
 * rendered window), so it needs a way in. Revealing a line is not a prop: the
 * target file may not be materialised, and scrolling to a placeholder resolves
 * nothing at all — so the surface has to materialise, flush, then scroll, and
 * only it knows how.
 */
export interface ReviewCodeViewHandle {
  revealLine: (path: string, lineNumber: number, side: 'old' | 'new') => void
  /** Bring a whole FILE into view — what the sidebar's changed-files list asks
   *  for. Distinct from revealLine because the caller has no line to aim at and
   *  a file's first diff line is not line 1. */
  revealFile: (path: string) => void
}

export interface ReviewCodeViewProps {
  wsId: string
  /** The branch-review summary: one row per changed file, in render order. */
  files: readonly GitDiff[]
  /** Hunk geometry for the same files. Order does not matter; it is joined by path. */
  outline: readonly FileOutline[]
  /** Scopes every read to ONE commit instead of the workspace's branch. It is
   *  part of the patch cache key as well as the request: the same path in the
   *  branch diff and in a commit diff are different content, and @pierre/diffs
   *  treats the cacheKey as the item's IDENTITY — a collision keeps the first
   *  one's parsed body and rendered height. */
  commit?: string
  /** False while the pane is hidden, which suspends fetching for it. */
  isActivePane?: boolean
  /** 'split' (default): two-column, side-by-side. 'unified': one column,
   *  old/new lines interleaved inline. Passed straight through to
   *  `@pierre/diffs`' own `diffStyle` option — both are already fully
   *  implemented by DiffHunksRenderer. */
  diffStyle?: 'split' | 'unified'
  /** Imperative handle for callers that must navigate the surface from
   *  outside it — find-in-diff resolves hits against the daemon, not the
   *  rendered window, so it cannot reach a line any other way. */
  surfaceRef?: React.Ref<ReviewCodeViewHandle>
  className?: string
}

/**
 * Whether a highlighting worker can actually be BUILT in this environment.
 *
 * `typeof Worker !== 'undefined'` used to be the test, and it is not one — it is
 * true in precisely the case that breaks. If the constructor then throws (a CSP
 * that forbids workers; an engine refusing a module worker served from the
 * app's custom scheme) the pool is already installed, every `workerFactory()`
 * call throws, the library turns that into unhandled rejections, and the
 * surface renders NOTHING. Not unhighlighted text — nothing at all, silently.
 *
 * So build one and throw it away. Highlighting is an optimisation; being able
 * to READ the diff is not. Skipping the provider leaves the pool context
 * undefined, which CodeView already treats as "highlight inline".
 *
 * This catches only a SYNCHRONOUS failure. A worker that loads and then never
 * answers is the build-time hazard above, gated by verify-worker-bundles.mjs.
 */
let workerPoolUsable: boolean | undefined
function canUseWorkerPool(): boolean {
  if (workerPoolUsable !== undefined) return workerPoolUsable
  if (typeof Worker === 'undefined') {
    workerPoolUsable = false
    return workerPoolUsable
  }
  try {
    // The chunk this fetches is the one the pool is about to fetch anyway, so
    // the probe costs a cache hit rather than a second download.
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

/**
 * Build the pool's worker.
 *
 * `?worker` names the package's worker module as the worker ENTRY, which is the
 * whole point: the previous form was a local one-line module that imported it
 * only for its side effects, and `@pierre/diffs` declares
 * `"sideEffects": ["dist/components/web-components.js"]` — every other file in
 * the package, worker.js included, is advertised as side-effect free. A
 * production bundler is therefore entitled to drop that import, and it did: the
 * emitted worker chunk was 0 BYTES.
 *
 * Nothing failed loudly. An empty worker script loads fine, installs no message
 * handler, and answers none of the pool's requests — so every highlight stayed
 * pending forever and CodeView rendered no rows at all. The review pane went
 * blank in packaged builds while dev, which serves the module unbundled and
 * runs the import, stayed perfect.
 *
 * Naming the real module as the entry removes the failure structurally: an
 * entry is not a side effect to be proven, it is the thing being emitted, so
 * there is nothing left to shake away. `scripts/verify-worker-bundles.mjs` runs
 * as `postbuild` and fails the build if a worker chunk is ever empty again.
 */
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

  const placeholders = useMemo(() => {
    const map = new Map<string, FileDiffMetadata>()
    for (const entry of entries) {
      if (entry.kind !== 'diff') continue
      map.set(entry.path, buildPlaceholderFileDiff(entry.file, entry.outline))
    }
    return map
  }, [entries])

  const annotations = useReviewAnnotations({ wsId })
  const { annotationsByPath, threadCounts, threadsFor } = annotations

  const items = useMemo<CodeViewItem<ReviewThread>[]>(
    () =>
      [...placeholders].map(([path, fileDiff]) => ({
        id: path,
        type: 'diff',
        fileDiff,
        annotations: annotationsByPath.get(path),
        version: 0,
      })),
    // `initialItems` is seeded once, so this only decides which threads exist at
    // MOUNT; useReviewPatchWindow republishes every later change.
    [annotationsByPath, placeholders],
  )
  const paths = useMemo(() => items.map((item) => item.id), [items])
  const signature = useMemo(() => signatureOf(paths), [paths])
  const lineCounts = useMemo(() => {
    const counts: Record<string, number> = {}
    for (const [path, fileDiff] of placeholders) counts[path] = fileDiff.unifiedLineCount
    return counts
  }, [placeholders])

  // PaneContainer renders only the active buffer, so a tab switch unmounts this
  // surface entirely and a return remounts it from scratch — same as
  // MarkdownPreview (use-preserved-scroll.ts's own doc). Keyed by wsId+commit
  // (not just wsId) so a branch review and a commit-diff tab on the same
  // workspace, or two different commit tabs, each keep their own offset.
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
    }),
    [diffStyle],
  )

  return (
    <div className={cn('flex h-full min-h-0 flex-col', className)}>
      {binaries.length > 0 ? <ReviewBinaryFiles entries={binaries} /> : null}
      <CodeView<ReviewThread>
        // Remounting on a changed file list is deliberate: CodeView seeds
        // `initialItems` once, and a different set of files is a different
        // document, not an update to this one.
        key={signature}
        ref={handleRef}
        containerRef={markScrollerRef}
        initialItems={items}
        options={options}
        onScroll={runWindow}
        onSelectedLinesChange={annotations.onSelectedLinesChange}
        className="min-h-0 flex-1 overflow-y-auto"
        // Annotations and header metadata are portalled into LIGHT dom, so
        // Tailwind and the app's CSS-var tokens apply to both.
        //
        // The cast narrows the library's file-or-diff annotation union: every
        // item this surface publishes is a diff, so a file annotation (one with
        // no side) cannot reach here.
        renderAnnotation={(annotation) =>
          annotations.renderAnnotation(annotation as ReviewAnnotation)
        }
        renderGutterUtility={annotations.renderGutterUtility}
        renderHeaderMetadata={(item) => {
          const count = threadCounts[item.id] ?? 0
          const state = patchStates[item.id]
          if (count === 0 && state == null) return null
          return (
            <span className="flex items-center gap-2">
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
