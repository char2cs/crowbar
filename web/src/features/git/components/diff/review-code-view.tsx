import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { ChatCircle, FileArchive, Image as ImageIcon } from '@phosphor-icons/react'
import { parsePatchFiles } from '@pierre/diffs'
import type {
  ChangeTypes,
  CodeViewItem,
  CodeViewOptions,
  FileDiffMetadata,
  Hunk,
} from '@pierre/diffs'
import { CodeView, WorkerPoolContextProvider } from '@pierre/diffs/react'
import type { CodeViewHandle } from '@pierre/diffs/react'
import { Button } from '@/components/ui/button'
import { getReviewPatch } from '@/features/git/api/review-window-api'
import type { FileOutline, HunkShape } from '@/features/git/api/review-window-api'
import { MAX_MATERIALIZED_LINES, planWindow } from '@/features/git/lib/patch-window'
import type { GitDiff } from '@/features/git/types/git-types'
import type { ReviewThread } from '@/features/workspace/stores/slices/branch-review-slice'
import { cn } from '@/utils/cn'
import ImageDiffViewer from './git-diff-image'
import { toAnnotationSide, useReviewAnnotations } from './use-review-annotations'
import type { ReviewAnnotation } from './use-review-annotations'

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

const IMAGE_EXTENSIONS = new Set([
  'apng',
  'avif',
  'bmp',
  'gif',
  'ico',
  'jpeg',
  'jpg',
  'png',
  'svg',
  'tif',
  'tiff',
  'webp',
])

// ── File classification ─────────────────────────────────────────────

/** How a changed file is rendered: as a diff, as an image, or as a bare row. */
export type ReviewFileKind = 'diff' | 'image' | 'binary'

export interface ReviewFileEntry {
  path: string
  kind: ReviewFileKind
  /** The summary row — status flags and ± counts, never line content. */
  file: GitDiff
  /** Hunk geometry for the path, absent only if the outline disagrees with the summary. */
  outline?: FileOutline
}

/**
 * Classify every changed file, preserving the summary's order.
 *
 * Binary-ness comes from the OUTLINE (git emits no `@@` headers for a binary
 * file, so the outline reports it directly) rather than from the summary, which
 * only carries counts. Image-ness is then decided by extension: the daemon does
 * not distinguish a PNG from a `.bin`, and the difference is purely about which
 * component can show it.
 */
export function partitionReviewFiles(
  files: readonly GitDiff[],
  outline: readonly FileOutline[],
): ReviewFileEntry[] {
  const byPath = new Map<string, FileOutline>()
  for (const entry of outline) {
    if (!byPath.has(entry.path)) byPath.set(entry.path, entry)
  }

  return files.map((file) => {
    const path = file.file_path
    const shape = byPath.get(path)
    const binary = shape?.isBinary ?? file.is_binary ?? false
    if (!binary) return { path, kind: 'diff', file, outline: shape }
    const image = file.is_image ?? isImagePath(path)
    return { path, kind: image ? 'image' : 'binary', file, outline: shape }
  })
}

function isImagePath(path: string): boolean {
  const dot = path.lastIndexOf('.')
  if (dot < 0) return false
  return IMAGE_EXTENSIONS.has(path.slice(dot + 1).toLowerCase())
}

// ── Placeholder geometry ────────────────────────────────────────────

/**
 * Build the content-free `FileDiffMetadata` a file occupies before its patch
 * arrives.
 *
 * `computeEstimatedDiffHeights` — what CodeView sizes an item with — reads only
 * each hunk's `splitLineCount` / `unifiedLineCount`, never a line of text. So a
 * hunk here carries its rendered row counts and an EMPTY `hunkContent`: the
 * virtualiser reserves the right space and the renderer draws nothing until the
 * real patch replaces it.
 *
 * The row counts need the split between context and changed lines, which a
 * `@@` header does not give — only `oldLines` and `newLines`. The summary's ±
 * counts close that gap exactly: across a whole file, context = Σ oldLines −
 * deletions, and distributing that across hunks (bounded per hunk by
 * `min(oldLines, newLines)`, the most context a hunk can hold) recovers each
 * hunk's shape. Without usable ± counts the estimate falls back to zero context
 * — the upper bound, because over-reserving costs a scrollbar that shrinks
 * while under-reserving costs a scroll that jumps.
 *
 * `isPartial` is always true on a placeholder. It is the library's flag for
 * "these lines are only what the patch showed", which switches off the
 * expand-unchanged machinery that would otherwise index into the (empty) line
 * arrays.
 */
export function buildPlaceholderFileDiff(
  file: GitDiff,
  outline: FileOutline | undefined,
): FileDiffMetadata {
  const shapes = outline?.hunks ?? []
  const additions = countOf(file.additions)
  const deletions = countOf(file.deletions)
  const hunks = buildPlaceholderHunks(shapes, distributeContext(shapes, deletions))

  // A capped outline stops at the server's per-file hunk limit, so its geometry
  // is a LOWER bound on the file — sizing from it alone under-reserves by
  // however much the cap cut. Every changed line renders one unified row, so
  // the summary's ± counts are a floor the outline must be topped up to.
  if (outline?.isPartial) {
    const missingAdditions = Math.max(0, additions - sumBy(hunks, (h) => h.additionLines))
    const missingDeletions = Math.max(0, deletions - sumBy(hunks, (h) => h.deletionLines))
    if (missingAdditions + missingDeletions > 0) {
      hunks.push(buildTailHunk(hunks, missingAdditions, missingDeletions))
    }
  }

  const name = outline?.path ?? file.file_path
  const prevName = outline?.oldPath ?? file.old_path
  return {
    name,
    prevName: prevName != null && prevName !== name ? prevName : undefined,
    type: changeTypeOf(file),
    hunks,
    splitLineCount: sumBy(hunks, (h) => h.splitLineCount),
    unifiedLineCount: sumBy(hunks, (h) => h.unifiedLineCount),
    isPartial: true,
    deletionLines: [],
    additionLines: [],
  }
}

/** A summary count, or 0 when it is absent or the binary sentinel (-1). */
function countOf(value: number | undefined): number {
  return typeof value === 'number' && value > 0 ? value : 0
}

function sumBy<T>(items: readonly T[], of: (item: T) => number): number {
  let total = 0
  for (const item of items) total += of(item)
  return total
}

function changeTypeOf(file: GitDiff): ChangeTypes {
  if (file.is_new) return 'new'
  if (file.is_deleted) return 'deleted'
  if (file.is_renamed) return 'rename-changed'
  return 'change'
}

/** Per-hunk context-line estimate; see `buildPlaceholderFileDiff`. */
function distributeContext(shapes: readonly HunkShape[], deletions: number): number[] {
  const caps = shapes.map((s) => Math.min(s.oldLines, s.newLines))
  const capTotal = sumBy(caps, (c) => c)
  if (capTotal === 0 || deletions === 0) return caps.map(() => 0)

  const oldTotal = sumBy(shapes, (s) => s.oldLines)
  const contextTotal = Math.min(Math.max(oldTotal - deletions, 0), capTotal)
  return caps.map((cap) => Math.round((contextTotal * cap) / capTotal))
}

function buildPlaceholderHunks(shapes: readonly HunkShape[], context: readonly number[]): Hunk[] {
  const hunks: Hunk[] = []
  let unifiedLineStart = 0
  let splitLineStart = 0
  let additionLineIndex = 0
  let deletionLineIndex = 0
  let previousOldEnd = 1

  for (let i = 0; i < shapes.length; i++) {
    const shape = shapes[i]
    const shared = Math.min(context[i] ?? 0, shape.oldLines, shape.newLines)
    const additionLines = shape.newLines - shared
    const deletionLines = shape.oldLines - shared

    hunks.push({
      collapsedBefore: Math.max(0, shape.oldStart - previousOldEnd),
      additionStart: shape.newStart,
      additionCount: shape.newLines,
      additionLines,
      additionLineIndex,
      deletionStart: shape.oldStart,
      deletionCount: shape.oldLines,
      deletionLines,
      deletionLineIndex,
      // Empty on purpose: this is the whole reason a placeholder is cheap. The
      // row counts below reserve the space; nothing draws until the patch lands.
      hunkContent: [],
      hunkSpecs: `@@ -${shape.oldStart},${shape.oldLines} +${shape.newStart},${shape.newLines} @@`,
      splitLineStart,
      splitLineCount: shared + Math.max(additionLines, deletionLines),
      unifiedLineStart,
      unifiedLineCount: shared + additionLines + deletionLines,
      noEOFCRDeletions: false,
      noEOFCRAdditions: false,
    })

    const last = hunks[hunks.length - 1]
    unifiedLineStart += last.unifiedLineCount
    splitLineStart += last.splitLineCount
    additionLineIndex += shape.newLines
    deletionLineIndex += shape.oldLines
    previousOldEnd = shape.oldStart + shape.oldLines
  }

  return hunks
}

/** The synthetic hunk standing in for everything a capped outline omitted. */
function buildTailHunk(hunks: readonly Hunk[], additions: number, deletions: number): Hunk {
  const previous = hunks[hunks.length - 1]
  return {
    collapsedBefore: 0,
    additionStart: previous != null ? previous.additionStart + previous.additionCount : 1,
    additionCount: additions,
    additionLines: additions,
    additionLineIndex: previous != null ? previous.additionLineIndex + previous.additionCount : 0,
    deletionStart: previous != null ? previous.deletionStart + previous.deletionCount : 1,
    deletionCount: deletions,
    deletionLines: deletions,
    deletionLineIndex: previous != null ? previous.deletionLineIndex + previous.deletionCount : 0,
    hunkContent: [],
    splitLineStart: previous != null ? previous.splitLineStart + previous.splitLineCount : 0,
    splitLineCount: Math.max(additions, deletions),
    unifiedLineStart: previous != null ? previous.unifiedLineStart + previous.unifiedLineCount : 0,
    unifiedLineCount: additions + deletions,
    noEOFCRDeletions: false,
    noEOFCRAdditions: false,
  }
}

// ── Patch parsing ───────────────────────────────────────────────────

/** Parse one file's unified patch. A header-only body yields a hunkless file
 *  rather than nothing, which is what lets the truncation banner still render. */
function parseSingleFilePatch(patch: string, cacheKey: string): FileDiffMetadata | undefined {
  try {
    for (const parsed of parsePatchFiles(patch, cacheKey)) {
      const file = parsed.files[0]
      if (file != null) return file
    }
  } catch {
    return undefined
  }
  return undefined
}

// ── The surface ─────────────────────────────────────────────────────

/**
 * The payload every annotation carries.
 *
 * Threads are the surface's only annotation, so the renderer's `LAnnotation`
 * type parameter IS `ReviewThread` — see `use-review-annotations.tsx`.
 */
type CodeViewInstance = ReturnType<CodeViewHandle<ReviewThread>['getInstance']>

/** Why a file's header is showing something other than its diff. */
type PatchState = 'truncated' | 'loading' | 'failed'

/** What a path holds right now. `failed` is remembered rather than forgotten so
 *  the planner does not refetch a broken path on every frame. */
interface HeldPatch {
  token: number
  state: 'loading' | 'ready' | 'failed'
}

export interface ReviewCodeViewProps {
  wsId: string
  /** The branch-review summary: one row per changed file, in render order. */
  files: readonly GitDiff[]
  /** Hunk geometry for the same files. Order does not matter; it is joined by path. */
  outline: readonly FileOutline[]
  /** False while the pane is hidden, which suspends fetching for it. */
  isActivePane?: boolean
  className?: string
}

/**
 * Whether this environment can run the highlighting pool at all.
 *
 * Without it the provider still constructs a pool, every `workerFactory()` call
 * throws, and the library reports that as an UNHANDLED REJECTION rather than as
 * the main-thread fallback it actually performs — green tests, exit code 1.
 * Skipping the provider leaves the pool context undefined, which CodeView
 * already treats as "highlight inline".
 */
const WORKERS_SUPPORTED = typeof Worker !== 'undefined'

export function ReviewCodeView(props: ReviewCodeViewProps) {
  if (!WORKERS_SUPPORTED) return <ReviewCodeViewSurface {...props} />
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
  return new Worker(new URL('../../lib/diffs-highlight-worker.ts', import.meta.url), {
    type: 'module',
  })
}

function ReviewCodeViewSurface({
  wsId,
  files,
  outline,
  isActivePane = true,
  className,
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
    // MOUNT; the effect below carries every later change.
    [annotationsByPath, placeholders],
  )
  const paths = useMemo(() => items.map((item) => item.id), [items])
  const signature = useMemo(() => signatureOf(paths), [paths])
  const lineCounts = useMemo(() => {
    const counts: Record<string, number> = {}
    for (const [path, fileDiff] of placeholders) counts[path] = fileDiff.unifiedLineCount
    return counts
  }, [placeholders])

  const [patchStates, setPatchStates] = useState<Record<string, PatchState>>({})

  const handleRef = useRef<CodeViewHandle<ReviewThread> | null>(null)
  const heldRef = useRef(new Map<string, HeldPatch>())
  const tokenRef = useRef(0)
  const versionRef = useRef(0)

  // Read inside imperative callbacks, never during render.
  const placeholdersRef = useRef(placeholders)
  const pathsRef = useRef(paths)
  const lineCountsRef = useRef(lineCounts)
  const activeRef = useRef(isActivePane)
  const annotationsRef = useRef(annotationsByPath)
  placeholdersRef.current = placeholders
  pathsRef.current = paths
  lineCountsRef.current = lineCounts
  activeRef.current = isActivePane
  annotationsRef.current = annotationsByPath

  /**
   * Republish one file's item.
   *
   * Every imperative update replaces the whole item, so the current annotations
   * have to ride along on all of them — omitting them anywhere silently drops
   * that file's threads the next time it is materialised or evicted.
   */
  const publishItem = useCallback((path: string, fileDiff: FileDiffMetadata) => {
    handleRef.current?.updateItem({
      id: path,
      type: 'diff',
      fileDiff,
      annotations: annotationsRef.current.get(path),
      version: ++versionRef.current,
    })
  }, [])

  const setPatchState = useCallback((path: string, state: PatchState | undefined) => {
    setPatchStates((prev) => {
      if (prev[path] === state) return prev
      const next = { ...prev }
      if (state == null) delete next[path]
      else next[path] = state
      return next
    })
  }, [])

  const evictPath = useCallback(
    (path: string) => {
      heldRef.current.delete(path)
      const placeholder = placeholdersRef.current.get(path)
      if (placeholder != null) publishItem(path, placeholder)
      setPatchState(path, undefined)
    },
    [publishItem, setPatchState],
  )

  const runWindowRef = useRef<() => void>(() => {})

  const materialize = useCallback(
    // `replan` is off only when the caller is about to move the viewport itself
    // (see `revealThread`): re-planning against the OLD scroll position would
    // evict the very file that is being navigated to.
    async (path: string, maxLines?: number, replan = true) => {
      const token = ++tokenRef.current
      heldRef.current.set(path, { token, state: 'loading' })
      try {
        const { patch, truncated } = await getReviewPatch(wsId, path, maxLines)
        // The path may have scrolled out of the window while the request was in
        // flight; the eviction that dropped it already replaced the item.
        if (heldRef.current.get(path)?.token !== token) return

        const fileDiff = parseSingleFilePatch(patch, `${wsId}:${path}`)
        if (fileDiff == null) {
          heldRef.current.set(path, { token, state: 'failed' })
          setPatchState(path, truncated ? 'truncated' : 'failed')
          return
        }

        heldRef.current.set(path, { token, state: 'ready' })
        publishItem(path, fileDiff)
        setPatchState(path, truncated ? 'truncated' : undefined)
      } catch {
        if (heldRef.current.get(path)?.token !== token) return
        // Kept in the held map on purpose: a forgotten failure is refetched on
        // the very next frame, forever.
        heldRef.current.set(path, { token, state: 'failed' })
        setPatchState(path, 'failed')
      } finally {
        // Materialising changed the item's height, so the window moved.
        if (replan) runWindowRef.current()
      }
    },
    [publishItem, setPatchState, wsId],
  )

  const runWindow = useCallback(() => {
    if (!activeRef.current) return
    const viewer = handleRef.current?.getInstance()
    const pathList = pathsRef.current
    if (viewer == null || pathList.length === 0) return

    const plan = planWindow({
      visible: visibleBand(viewer, pathList),
      total: pathList.length,
      materialized: [...heldRef.current.keys()],
      paths: pathList,
      lineCounts: lineCountsRef.current,
    })
    for (const path of plan.evict) evictPath(path)
    for (const path of plan.fetch) void materialize(path)
  }, [evictPath, materialize])
  runWindowRef.current = runWindow

  const expandTruncated = useCallback(
    (path: string) => {
      setPatchState(path, 'loading')
      // maxLines <= 0 is the engine's "unlimited" — the uncapped refetch.
      void materialize(path, 0)
    },
    [materialize, setPatchState],
  )

  // A retry re-runs the CAPPED request. Retrying uncapped would answer a failed
  // fetch of an ordinary file by asking for the most expensive thing available.
  const retryPatch = useCallback(
    (path: string) => {
      setPatchState(path, 'loading')
      void materialize(path)
    },
    [materialize, setPatchState],
  )

  /**
   * Scroll to a thread, materialising its file first.
   *
   * The order is the whole point. A file the window has not loaded is a
   * placeholder with no lines in it, and `scrollTo({type:'line'})` against one
   * resolves no position at all — it warns and does nothing, leaving the reader
   * exactly where they were with no indication anything was attempted.
   */
  const revealThread = useCallback(
    async (thread: ReviewThread) => {
      const path = thread.filePath
      if (heldRef.current.get(path)?.state !== 'ready') {
        setPatchState(path, 'loading')
        await materialize(path, undefined, false)
      }
      // `updateItem` only QUEUES a render, while `scrollTo` resolves its
      // destination eagerly — so without flushing, the scroll is computed
      // against the placeholder that was just replaced, finds no such line, and
      // silently does nothing.
      handleRef.current?.getInstance()?.render(true)
      handleRef.current?.scrollTo({
        type: 'line',
        id: path,
        lineNumber: thread.lineNumber,
        side: toAnnotationSide(thread.side),
        align: 'center',
      })
    },
    [materialize, setPatchState],
  )

  const revealFirstThread = useCallback(
    (path: string) => {
      const [first] = threadsFor(path)
      if (first != null) void revealThread(first)
    },
    [revealThread, threadsFor],
  )

  // A changed file list remounts CodeView (see the `key` below), which discards
  // every item — so what this component believes it is holding has to go with
  // them, or the planner never fetches those paths again.
  useEffect(() => {
    heldRef.current.clear()
    setPatchStates((prev) => (Object.keys(prev).length === 0 ? prev : {}))
  }, [signature])

  // The same file list with different geometry is NOT a remount: the summary
  // refetches on every git-status tick, and a file whose ± counts moved has a
  // new reserved height that only its placeholder carries. Republish it for
  // every file that is still showing a placeholder; a materialised file holds
  // real content and its height is measured, not estimated.
  useEffect(() => {
    const handle = handleRef.current
    if (handle == null) return
    for (const [path, placeholder] of placeholders) {
      if (heldRef.current.get(path)?.state === 'ready') continue
      const current = handle.getItem(path)
      if (current == null || current.type !== 'diff') continue
      if (
        current.fileDiff.unifiedLineCount === placeholder.unifiedLineCount &&
        current.fileDiff.splitLineCount === placeholder.splitLineCount
      ) {
        continue
      }
      publishItem(path, placeholder)
    }
  }, [placeholders, publishItem])

  // Threads arrive over the WS stream at any moment, entirely independently of
  // the diff. An item's annotations are part of the item, so a new, edited,
  // resolved or deleted thread has to be republished onto whatever that file is
  // currently showing — placeholder or real patch, it keeps its own geometry.
  useEffect(() => {
    const handle = handleRef.current
    if (handle == null) return
    for (const path of pathsRef.current) {
      const current = handle.getItem(path)
      if (current == null || current.type !== 'diff') continue
      const next = annotationsByPath.get(path)
      if (sameAnnotations(current.annotations, next)) continue
      handle.updateItem({
        id: path,
        type: 'diff',
        fileDiff: current.fileDiff,
        annotations: next,
        version: ++versionRef.current,
      })
    }
  }, [annotationsByPath])

  // The band is re-planned whenever the layout it depends on can have moved:
  // on mount, when the file list changes, and on every scroll.
  useEffect(() => {
    runWindow()
  }, [runWindow, items, isActivePane])

  useEffect(() => {
    const held = heldRef.current
    return () => held.clear()
  }, [])

  const options = useMemo<CodeViewOptions<ReviewThread>>(
    () => ({
      stickyHeaders: true,
      tokenizeMaxLineLength: REVIEW_TOKENIZE_MAX_LINE_LENGTH,
      tokenizeMaxLength: REVIEW_TOKENIZE_MAX_LENGTH,
      // Selecting a line range is how a comment is started, and the hovered "+"
      // is how a single-line one is. Both feed the same draft.
      enableLineSelection: true,
      enableGutterUtility: true,
    }),
    [],
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
        containerRef={markScroller}
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

/**
 * How many review threads a file carries, on its header.
 *
 * The threads store is always loaded; the diff is not. A thread anchored in a
 * file the window has left as a placeholder has no line to attach to and would
 * otherwise be invisible — so the header says it is there, and clicking loads
 * the file and goes to it.
 */
function FileThreadCount({
  path,
  count,
  onReveal,
}: {
  path: string
  count: number
  onReveal: (path: string) => void
}) {
  if (count <= 0) return null
  return (
    <Button
      variant="ghost"
      size="xs"
      aria-label={count === 1 ? '1 comment' : `${count} comments`}
      onClick={() => onReveal(path)}
    >
      <ChatCircle />
      {count}
    </Button>
  )
}

/** Value-compare two annotation lists; they are rebuilt on every thread tick. */
function sameAnnotations(
  a: readonly ReviewAnnotation[] | undefined,
  b: readonly ReviewAnnotation[] | undefined,
): boolean {
  if (a === b) return true
  if (a == null || b == null) return (a?.length ?? 0) === (b?.length ?? 0)
  if (a.length !== b.length) return false
  return a.every((annotation, i) => {
    const other = b[i]
    return (
      annotation.side === other.side &&
      annotation.lineNumber === other.lineNumber &&
      annotation.metadata === other.metadata
    )
  })
}

/** Tags the CodeView's own scroll container so styling and tests can find it. */
function markScroller(node: HTMLDivElement | null): void {
  if (node != null) node.dataset.reviewCodeViewScroller = ''
}

/**
 * What a file's header says when its body is not the whole story.
 *
 * The case this exists for: the fixture's monster file is a SINGLE 420k-line
 * hunk, so under the server's default cap nothing fits and the response is a
 * 142-byte header-only patch. Rendered naively that reads as "this file has no
 * changes" — the one thing it must never say.
 */
function PatchStateNotice({
  path,
  state,
  onExpand,
  onRetry,
}: {
  path: string
  state: PatchState | undefined
  onExpand: (path: string) => void
  onRetry: (path: string) => void
}) {
  if (state == null) return null
  if (state === 'loading') {
    return <span className="ui-text-xs text-muted-foreground">Loading…</span>
  }
  if (state === 'failed') {
    return (
      <span className="flex items-center gap-2 ui-text-xs text-muted-foreground">
        Could not load this diff
        <Button variant="ghost" compact onClick={() => onRetry(path)}>
          Retry
        </Button>
      </span>
    )
  }
  return (
    <span className="flex items-center gap-2 ui-text-xs text-muted-foreground">
      Diff truncated
      <Button variant="ghost" compact onClick={() => onExpand(path)}>
        Show all
      </Button>
    </span>
  )
}

// ── Binary files ────────────────────────────────────────────────────

/**
 * Binary files, kept out of the diff renderer entirely.
 *
 * They sit in their own block rather than inline in the CodeView because
 * CodeView renders exactly two things — a file and a diff — and a binary is
 * neither. Rows start collapsed so a review with fifty images neither decodes
 * fifty images nor blows the pane's height open.
 */
function ReviewBinaryFiles({ entries }: { entries: readonly ReviewFileEntry[] }) {
  return (
    <section
      aria-label="Binary files"
      className="max-h-[50%] shrink-0 overflow-y-auto border-border border-b"
    >
      {entries.map((entry) => (
        <ReviewBinaryFile key={entry.path} entry={entry} />
      ))}
    </section>
  )
}

const BINARY_ROW_CLASS = 'flex w-full items-center gap-2 px-3 py-2 text-left ui-text-sm'

function ReviewBinaryFile({ entry }: { entry: ReviewFileEntry }) {
  const [open, setOpen] = useState(false)

  // A non-image binary has nothing to disclose, so it is a row and not a
  // control — a disabled button would announce itself as one that is broken.
  if (entry.kind !== 'image') {
    return (
      <div className="border-border/70 border-b last:border-b-0">
        <div className={BINARY_ROW_CLASS}>
          <BinaryRowLabel path={entry.path} label="Binary file" icon="binary" />
        </div>
      </div>
    )
  }

  return (
    <div className="border-border/70 border-b last:border-b-0">
      <button
        type="button"
        onClick={() => setOpen((prev) => !prev)}
        aria-expanded={open}
        className={cn(BINARY_ROW_CLASS, 'hover:bg-muted/30')}
      >
        <BinaryRowLabel path={entry.path} label="Image" icon="image" />
      </button>
      {open ? (
        <div className="h-96 border-border/70 border-t">
          <ImageDiffViewer diff={entry.file} fileName={entry.path} onClose={() => setOpen(false)} />
        </div>
      ) : null}
    </div>
  )
}

function BinaryRowLabel({
  path,
  label,
  icon,
}: {
  path: string
  label: string
  icon: 'image' | 'binary'
}) {
  return (
    <>
      {icon === 'image' ? (
        <ImageIcon className="shrink-0 text-muted-foreground" />
      ) : (
        <FileArchive className="shrink-0 text-muted-foreground" />
      )}
      <span className="min-w-0 flex-1 truncate editor-font">{path}</span>
      <span className="shrink-0 text-muted-foreground ui-text-xs">{label}</span>
    </>
  )
}

// ── Viewport geometry ───────────────────────────────────────────────

/**
 * The inclusive index range of files currently on screen.
 *
 * Derived from the viewer's own item tops rather than from its rendered range,
 * because `onScroll` fires BEFORE the next render pass: reading the render
 * range there would plan the window one frame behind every scroll. Item tops
 * are monotonic, so a binary search answers in O(log files).
 */
function visibleBand(
  viewer: NonNullable<CodeViewInstance>,
  paths: readonly string[],
): { first: number; last: number } {
  const scrollTop = viewer.getScrollTop()
  const viewportHeight = Math.max(viewer.getHeight(), 1)
  const first = lastIndexAbove(viewer, paths, scrollTop)
  return { first, last: Math.max(first, lastIndexAbove(viewer, paths, scrollTop + viewportHeight)) }
}

/** Index of the last file whose top edge is at or above `y`. */
function lastIndexAbove(
  viewer: NonNullable<CodeViewInstance>,
  paths: readonly string[],
  y: number,
): number {
  let low = 0
  let high = paths.length - 1
  let best = 0
  while (low <= high) {
    const mid = (low + high) >> 1
    const top = viewer.getTopForItem(paths[mid])
    if (top == null) return best
    if (top <= y) {
      best = mid
      low = mid + 1
    } else {
      high = mid - 1
    }
  }
  return best
}

/** A short, stable identity for an ordered path list (FNV-1a). */
function signatureOf(paths: readonly string[]): string {
  let hash = 0x811c9dc5
  for (const path of paths) {
    for (let i = 0; i < path.length; i++) {
      hash ^= path.charCodeAt(i)
      hash = Math.imul(hash, 0x01000193)
    }
    hash ^= 0x2f
    hash = Math.imul(hash, 0x01000193)
  }
  return `${paths.length}:${(hash >>> 0).toString(36)}`
}
