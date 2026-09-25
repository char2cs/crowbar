import { useCallback, useEffect, useImperativeHandle, useRef, useState, type Ref } from 'react'
import type { CodeViewItem, FileDiffMetadata } from '@pierre/diffs'
import type { CodeViewHandle } from '@pierre/diffs/react'
import { getReviewPatch } from '@/features/git/api/review-window-api'
import { planWindow } from '@/features/git/lib/patch-window'
import { parseSingleFilePatch, patchCacheKey } from '@/features/git/lib/review-placeholder'
import type { ReviewThread } from '@/features/workspace/stores/slices/branch-review-slice'
import type { ReviewCodeViewHandle } from './review-code-view'
import type { PatchState } from './review-patch-header'
import { toAnnotationSide } from './use-review-annotations'
import type { ReviewAnnotation } from './use-review-annotations'

/**
 * The payload every annotation carries.
 *
 * Threads are the surface's only annotation, so the renderer's `LAnnotation`
 * type parameter IS `ReviewThread` — see `use-review-annotations.tsx`.
 */
type CodeViewInstance = ReturnType<CodeViewHandle<ReviewThread, undefined>['getInstance']>

/** What a path holds right now. `failed` is remembered rather than forgotten so
 *  the planner does not refetch a broken path on every frame. */
interface HeldPatch {
  token: number
  state: 'loading' | 'ready' | 'failed'
}

interface ReviewPatchWindowOptions {
  wsId: string
  commit?: string
  isActivePane: boolean
  placeholders: Map<string, FileDiffMetadata>
  items: CodeViewItem<ReviewThread>[]
  paths: string[]
  lineCounts: Record<string, number>
  signature: string
  annotationsByPath: ReadonlyMap<string, ReviewAnnotation[]>
  surfaceRef?: Ref<ReviewCodeViewHandle>
}

/**
 * The patch window behind the review surface: which files hold real patch
 * text right now, fetching as the reader approaches a file and evicting back
 * to its placeholder once they have moved on, plus reveal (materialise, flush,
 * scroll) for threads, lines and files.
 */
export function useReviewPatchWindow({
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
}: ReviewPatchWindowOptions) {
  const [patchStates, setPatchStates] = useState<Record<string, PatchState>>({})
  const handleRef = useRef<CodeViewHandle<ReviewThread, undefined> | null>(null)
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
  /** The last fileDiff published per path, so a header repaint can reuse it. */
  const publishedRef = useRef(new Map<string, FileDiffMetadata>())

  const publishItem = useCallback((path: string, fileDiff: FileDiffMetadata) => {
    publishedRef.current.set(path, fileDiff)
    handleRef.current?.updateItem({
      id: path,
      type: 'diff',
      fileDiff,
      annotations: annotationsRef.current.get(path),
      version: ++versionRef.current,
    })
  }, [])

  const setPatchState = useCallback(
    (path: string, state: PatchState | undefined) => {
      let changed = false
      setPatchStates((prev) => {
        if (prev[path] === state) return prev
        changed = true
        const next = { ...prev }
        if (state == null) delete next[path]
        else next[path] = state
        return next
      })
      // The notice ("Diff truncated / Show all", "Retry") renders through
      // renderHeaderMetadata, and the library CACHES a file's header HTML — a
      // React state change alone never repaints it, so the notice could never
      // appear. Republishing with a bumped version invalidates that cache.
      if (!changed) return
      const current = publishedRef.current.get(path) ?? placeholdersRef.current.get(path)
      if (current != null) publishItem(path, current)
    },
    [publishItem],
  )

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
        const { patch, truncated } = await getReviewPatch({ wsId, commit }, path, maxLines)
        // The path may have scrolled out of the window while the request was in
        // flight; the eviction that dropped it already replaced the item.
        if (heldRef.current.get(path)?.token !== token) return

        const fileDiff = parseSingleFilePatch(patch, patchCacheKey(wsId, commit, path, patch))
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
    [publishItem, setPatchState, wsId, commit],
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

  /** The same materialise → flush → scroll dance, addressed by line rather
   *  than by thread, for callers outside this component (find-in-diff). */
  const revealLine = useCallback(
    async (path: string, lineNumber: number, side: 'old' | 'new') => {
      if (heldRef.current.get(path)?.state !== 'ready') {
        setPatchState(path, 'loading')
        await materialize(path, undefined, false)
      }
      handleRef.current?.getInstance()?.render(true)
      handleRef.current?.scrollTo({
        type: 'line',
        id: path,
        lineNumber,
        side: side === 'old' ? 'deletions' : 'additions',
        align: 'center',
      })
    },
    [materialize, setPatchState],
  )

  const revealFile = useCallback(
    async (path: string) => {
      // Materialise first: a placeholder's reserved height is an ESTIMATE, so
      // scrolling to it and then letting the real patch resize it would leave
      // the reader somewhere near the file rather than at it.
      if (heldRef.current.get(path)?.state !== 'ready') {
        setPatchState(path, 'loading')
        await materialize(path, undefined, false)
      }
      handleRef.current?.getInstance()?.render(true)
      handleRef.current?.scrollTo({ type: 'item', id: path, align: 'start' })
    },
    [materialize, setPatchState],
  )

  useImperativeHandle(
    surfaceRef,
    () => ({
      revealLine: (path, lineNumber, side) => {
        void revealLine(path, lineNumber, side)
      },
      revealFile: (path) => {
        void revealFile(path)
      },
    }),
    [revealLine, revealFile],
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

  return { handleRef, patchStates, runWindow, expandTruncated, retryPatch, revealThread }
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
