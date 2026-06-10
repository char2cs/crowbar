import { useCallback, useEffect, useRef, useState } from 'react'
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
  const buffers = useStore(workspaceStore, (s) => s.buffers)
  const activeBufferId = useStore(
    workspaceStore,
    (s) => s.paneActions.getActivePane()?.activeBufferId ?? null,
  )
  const activeBuffer = buffers.find((b) => b.id === activeBufferId) || null
  const updateBufferContent = (
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
  }
  const closeBuffer = (id: string) => workspaceStore.getState().bufferActions.closeBuffer(id)
  const rootFolderPath = useFileSystemStore((s) => s.rootFolderPath)

  const [isLoading, setIsLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const isRefreshing = useRef(false)

  const rawDiffData: GitDiff | MultiFileDiff | null =
    (activeBuffer?.type === 'diff' && activeBuffer.diffData) ||
    (activeBuffer?.type === 'diff' && activeBuffer.content
      ? (() => {
          try {
            return JSON.parse(activeBuffer.content) as GitDiff | MultiFileDiff
          } catch {
            return null
          }
        })()
      : null)

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
