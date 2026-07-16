import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useStore } from 'zustand'
import { useWorkspaceStore } from '@/features/workspace/stores/workspace-context'
import { useFileSystemStore } from '@/features/file-system/controllers/store'
import { getFileDiff } from '../api/git-diff-api'
import type { MultiFileDiff } from '../types/git-diff-types'
import type { GitDiff } from '../types/git-types'
import { getDiffBufferFilePath } from '../utils/diff-buffer-path'

interface UseDiffDataReturn {
  diff: GitDiff | null
  rawDiffData: GitDiff | MultiFileDiff | null
  filePath: string | null
  isStaged: boolean
  isLoading: boolean
  error: string | null
  refresh: () => Promise<void>
  switchToView: (viewType: 'staged' | 'unstaged') => void
}

export const useDiffData = (): UseDiffDataReturn => {
  const workspaceStore = useWorkspaceStore()
  const activeBufferId = useStore(
    workspaceStore,
    (s) => s.paneActions.getActivePane()?.activeBufferId ?? null,
  )
  // Subscribe to ONLY the active buffer, not the whole `buffers` array. immer's
  // structural sharing keeps every other buffer's reference identical, so a
  // content flush on some OTHER buffer no longer re-renders DiffViewer or
  // re-runs the find + JSON.parse below. This re-renders only when the active
  // buffer itself changes identity (its content/diffData actually moved).
  const activeBuffer = useStore(
    workspaceStore,
    (s) => s.buffers.find((b) => b.id === activeBufferId) ?? null,
  )
  // Stable identity: refresh() depends on these, and refresh() is itself a
  // dependency of the git-status-changed listener effect — unstable references
  // would re-subscribe that window listener on every render.
  const updateBufferContent = useCallback(
    (
      bufferId: string,
      content: string,
      _markDirty: boolean,
      diffData?:
        | import('../types/git-diff-types').MultiFileDiff
        | import('../types/git-types').GitDiff,
    ) => {
      workspaceStore.setState((state) => ({
        ...state,
        buffers: state.buffers.map((b) =>
          b.id === bufferId && b.type === 'diff'
            ? { ...b, content, ...(diffData !== undefined ? { diffData } : {}) }
            : b,
        ),
      }))
    },
    [workspaceStore],
  )
  const closeBuffer = useCallback(
    (id: string) => workspaceStore.getState().bufferActions.closeBuffer(id),
    [workspaceStore],
  )
  const rootFolderPath = useFileSystemStore((s) => s.rootFolderPath)

  const [isLoading, setIsLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const isRefreshing = useRef(false)

  // Keep the JSON.parse fallback out of the render path: it now runs only when
  // the active buffer's identity changes (memoized on it), not on every
  // unrelated buffer churn tick — the id-scoped subscription above already
  // stops those from re-rendering, and this stops the parse from re-running
  // even when this hook does re-render for another reason.
  const rawDiffData = useMemo<GitDiff | MultiFileDiff | null>(() => {
    if (activeBuffer?.type !== 'diff') return null
    if (activeBuffer.diffData) return activeBuffer.diffData
    if (activeBuffer.content) {
      try {
        return JSON.parse(activeBuffer.content) as GitDiff | MultiFileDiff
      } catch {
        return null
      }
    }
    return null
  }, [activeBuffer])

  const diff = rawDiffData && 'file_path' in rawDiffData ? rawDiffData : null

  const stagedMatch = activeBuffer?.path.match(/^diff:\/\/(staged|unstaged)\/(.+)$/)
  const isStaged = stagedMatch?.[1] === 'staged'
  const isWorkingTreeFileDiff = Boolean(stagedMatch)
  const filePath = getDiffBufferFilePath(activeBuffer?.path)

  const switchToView = useCallback(
    (viewType: 'staged' | 'unstaged') => {
      if (!filePath) return

      const encodedPath = encodeURIComponent(filePath)
      const newVirtualPath = `diff://${viewType}/${encodedPath}`
      const displayName = `${filePath.split('/').pop()} (${viewType})`

      getFileDiff(rootFolderPath!, filePath, viewType === 'staged').then((newDiff) => {
        if (newDiff && newDiff.lines.length > 0) {
          workspaceStore.getState().bufferActions.openContent({
            type: 'diff',
            path: newVirtualPath,
            name: displayName,
            content: '',
            diffData: newDiff,
          })
        }
      })
    },
    [filePath, rootFolderPath, workspaceStore],
  )

  const refresh = useCallback(async () => {
    if (
      !isWorkingTreeFileDiff ||
      !rootFolderPath ||
      !filePath ||
      !activeBuffer ||
      isRefreshing.current
    ) {
      return
    }

    isRefreshing.current = true
    setIsLoading(true)
    setError(null)

    try {
      const currentViewDiff = await getFileDiff(rootFolderPath, filePath, isStaged)

      if (currentViewDiff && currentViewDiff.lines.length > 0) {
        updateBufferContent(activeBuffer.id, '', false, currentViewDiff)
      } else {
        const otherViewDiff = await getFileDiff(rootFolderPath, filePath, !isStaged)

        if (otherViewDiff && otherViewDiff.lines.length > 0) {
          switchToView(isStaged ? 'unstaged' : 'staged')
          setTimeout(() => closeBuffer(activeBuffer.id), 100)
        } else {
          closeBuffer(activeBuffer.id)
        }
      }
    } catch (err) {
      console.error('Failed to refresh diff:', err)
      setError(err instanceof Error ? err.message : 'Failed to refresh diff')
    } finally {
      setIsLoading(false)
      isRefreshing.current = false
    }
  }, [
    rootFolderPath,
    filePath,
    isStaged,
    isWorkingTreeFileDiff,
    activeBuffer,
    updateBufferContent,
    closeBuffer,
    switchToView,
  ])

  useEffect(() => {
    const handleGitStatusChanged = async () => {
      if (!isWorkingTreeFileDiff || !rootFolderPath || !filePath || !activeBuffer) return

      if (isRefreshing.current) return

      setTimeout(() => {
        if (!isRefreshing.current) {
          refresh()
        }
      }, 50)
    }

    window.addEventListener('git-status-changed', handleGitStatusChanged)
    return () => {
      window.removeEventListener('git-status-changed', handleGitStatusChanged)
    }
  }, [refresh, rootFolderPath, filePath, activeBuffer, isWorkingTreeFileDiff])

  return {
    diff,
    rawDiffData,
    filePath,
    isStaged,
    isLoading,
    error,
    refresh,
    switchToView,
  }
}
