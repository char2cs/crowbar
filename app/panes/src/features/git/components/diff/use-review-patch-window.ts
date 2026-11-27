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
import { onGitStatusChanged } from '@/features/git/stores/git-refresh'
import type { ReviewAnnotation } from './use-review-annotations'

type CodeViewInstance = ReturnType<CodeViewHandle<ReviewThread, undefined>['getInstance']>

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
  viewedPaths: ReadonlySet<string>
  signature: string
  annotationsByPath: ReadonlyMap<string, ReviewAnnotation[]>
  surfaceRef?: Ref<ReviewCodeViewHandle>
}

export function useReviewPatchWindow({
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
}: ReviewPatchWindowOptions) {
  const [patchStates, setPatchStates] = useState<Record<string, PatchState>>({})
  const handleRef = useRef<CodeViewHandle<ReviewThread, undefined> | null>(null)
  const heldRef = useRef(new Map<string, HeldPatch>())
  const tokenRef = useRef(0)
  const versionRef = useRef(0)

  const placeholdersRef = useRef(placeholders)
  const pathsRef = useRef(paths)
  const lineCountsRef = useRef(lineCounts)
  const viewedRef = useRef(viewedPaths)
  const activeRef = useRef(isActivePane)
  const annotationsRef = useRef(annotationsByPath)
  placeholdersRef.current = placeholders
  pathsRef.current = paths
  lineCountsRef.current = lineCounts
  viewedRef.current = viewedPaths
  activeRef.current = isActivePane
  annotationsRef.current = annotationsByPath

  const publishedRef = useRef(new Map<string, FileDiffMetadata>())

  const publishItem = useCallback((path: string, fileDiff: FileDiffMetadata) => {
    publishedRef.current.set(path, fileDiff)
    handleRef.current?.updateItem({
      id: path,
      type: 'diff',
      fileDiff,
      annotations: annotationsRef.current.get(path),
      collapsed: viewedRef.current.has(path),
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
    async (path: string, maxLines?: number, replan = true) => {
      if (viewedRef.current.has(path)) return
      const token = ++tokenRef.current
      heldRef.current.set(path, { token, state: 'loading' })
      try {
        const { patch, truncated } = await getReviewPatch({ wsId, commit }, path, maxLines)
        if (heldRef.current.get(path)?.token !== token) return
        if (viewedRef.current.has(path)) {
          evictPath(path)
          return
        }

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
        heldRef.current.set(path, { token, state: 'failed' })
        setPatchState(path, 'failed')
      } finally {
        if (replan) runWindowRef.current()
      }
    },
    [evictPath, publishItem, setPatchState, wsId, commit],
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
      viewed: viewedRef.current,
    })
    for (const path of plan.evict) evictPath(path)
    for (const path of plan.fetch) void materialize(path)
  }, [evictPath, materialize])
  runWindowRef.current = runWindow

  const expandTruncated = useCallback(
    (path: string) => {
      setPatchState(path, 'loading')
      void materialize(path, 0)
    },
    [materialize, setPatchState],
  )

  const retryPatch = useCallback(
    (path: string) => {
      setPatchState(path, 'loading')
      void materialize(path)
    },
    [materialize, setPatchState],
  )

  const scrollToViewedHeader = useCallback((path: string): boolean => {
    if (!viewedRef.current.has(path)) return false
    handleRef.current?.scrollTo({ type: 'item', id: path, align: 'start' })
    return true
  }, [])

  const revealThread = useCallback(
    async (thread: ReviewThread) => {
      const path = thread.filePath
      if (scrollToViewedHeader(path)) return
      if (heldRef.current.get(path)?.state !== 'ready') {
        setPatchState(path, 'loading')
        await materialize(path, undefined, false)
      }
      handleRef.current?.getInstance()?.render(true)
      handleRef.current?.scrollTo({
        type: 'line',
        id: path,
        lineNumber: thread.lineNumber,
        side: toAnnotationSide(thread.side),
        align: 'center',
      })
    },
    [materialize, scrollToViewedHeader, setPatchState],
  )

  const revealLine = useCallback(
    async (path: string, lineNumber: number, side: 'old' | 'new') => {
      if (scrollToViewedHeader(path)) return
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
    [materialize, scrollToViewedHeader, setPatchState],
  )

  const revealFile = useCallback(
    async (path: string) => {
      if (scrollToViewedHeader(path)) return
      if (heldRef.current.get(path)?.state !== 'ready') {
        setPatchState(path, 'loading')
        await materialize(path, undefined, false)
      }
      handleRef.current?.getInstance()?.render(true)
      handleRef.current?.scrollTo({ type: 'item', id: path, align: 'start' })
    },
    [materialize, scrollToViewedHeader, setPatchState],
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

  useEffect(() => {
    heldRef.current.clear()
    setPatchStates((prev) => (Object.keys(prev).length === 0 ? prev : {}))
  }, [signature])

  useEffect(() => {
    if (commit) return
    return onGitStatusChanged(wsId, () => {
      for (const [path, held] of heldRef.current) {
        if (held.state === 'ready') void materialize(path)
      }
    })
  }, [wsId, commit, materialize])

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
        collapsed: current.collapsed,
        version: ++versionRef.current,
      })
    }
  }, [annotationsByPath])

  const collapsedRef = useRef<ReadonlySet<string>>(new Set())
  useEffect(() => {
    const handle = handleRef.current
    const previous = collapsedRef.current
    collapsedRef.current = viewedPaths
    if (handle == null) return
    for (const path of [...previous, ...viewedPaths]) {
      const collapsed = viewedPaths.has(path)
      if (previous.has(path) === collapsed) continue
      const current = handle.getItem(path)
      if (current == null || current.type !== 'diff') continue
      handle.updateItem({ ...current, collapsed, version: ++versionRef.current })
    }
  }, [viewedPaths])

  useEffect(() => {
    runWindow()
  }, [runWindow, items, isActivePane])

  useEffect(() => {
    const held = heldRef.current
    return () => held.clear()
  }, [])

  return { handleRef, patchStates, runWindow, expandTruncated, retryPatch, revealThread }
}

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

function visibleBand(
  viewer: NonNullable<CodeViewInstance>,
  paths: readonly string[],
): { first: number; last: number } {
  const scrollTop = viewer.getScrollTop()
  const viewportHeight = Math.max(viewer.getHeight(), 1)
  const first = lastIndexAbove(viewer, paths, scrollTop)
  return { first, last: Math.max(first, lastIndexAbove(viewer, paths, scrollTop + viewportHeight)) }
}

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
