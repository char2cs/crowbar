import { useCallback } from 'react'
import { primitiveAlert } from '@/components/ui/primitive-dialog-service'
import { getActiveWorkspaceStoreRef } from '@/features/workspace/stores/workspace-store-ref'
import { getCommitDiff, getFileDiff, getRefDiff, getStashDiff } from '../api/git-diff-api'
import { useGitStore } from '../stores/git-store'
import type { MultiFileDiff } from '../types/git-diff-types'
import type { GitFile } from '../types/git-types'
import { countDiffStats } from '../utils/git-diff-helpers'

interface UseGitDiffHandlersProps {
  activeRepoPath: string | null
  visibleGitFiles: GitFile[]
  onFileSelect?: (path: string, isDir: boolean) => void
}

export function useGitDiffHandlers({
  activeRepoPath,
  visibleGitFiles,
  onFileSelect,
}: UseGitDiffHandlersProps) {
  const handleOpenOriginalFile = useCallback(
    async (filePath: string) => {
      if (!activeRepoPath || !onFileSelect) return
      try {
        let actualFilePath = filePath
        if (filePath.includes(' -> ')) {
          actualFilePath = filePath.split(' -> ')[1].trim()
        }
        if (actualFilePath.startsWith('"') && actualFilePath.endsWith('"')) {
          actualFilePath = actualFilePath.slice(1, -1)
        }
        onFileSelect(`${activeRepoPath}/${actualFilePath}`, false)
      } catch (error) {
        console.error('Error opening file:', error)
        await primitiveAlert(`Failed to open file ${filePath}:\n${error}`, 'Open File')
      }
    },
    [activeRepoPath, onFileSelect],
  )

  const handleViewFileDiff = useCallback(
    async (filePath: string, staged = false) => {
      if (!activeRepoPath || !onFileSelect) return
      try {
        let actualFilePath = filePath
        if (filePath.includes(' -> ')) {
          const parts = filePath.split(' -> ')
          actualFilePath = staged ? parts[1].trim() : parts[0].trim()
        }
        if (actualFilePath.startsWith('"') && actualFilePath.endsWith('"')) {
          actualFilePath = actualFilePath.slice(1, -1)
        }

        const file = useGitStore
          .getState()
          .gitStatus?.files.find((f: GitFile) => f.path === actualFilePath)
        if (file && file.status === 'untracked' && !staged) {
          await handleOpenOriginalFile(actualFilePath)
          return
        }

        const diff = await getFileDiff(activeRepoPath, actualFilePath, staged)
        if (diff && (diff.lines.length > 0 || diff.is_image)) {
          const selectedFileKey = `${staged ? 'staged' : 'unstaged'}:${actualFilePath}`
          const { additions, deletions } = countDiffStats([diff])

          const initialMultiDiff: MultiFileDiff = {
            title: 'Uncommitted Changes',
            commitHash: 'working-tree',
            files: [diff],
            totalFiles: 1,
            totalAdditions: additions,
            totalDeletions: deletions,
            fileKeys: [selectedFileKey],
            initiallyExpandedFileKey: selectedFileKey,
            isLoading: true,
          }

          const virtualPath = 'diff://working-tree/all-files'
          const bufferId =
            getActiveWorkspaceStoreRef()?.getState().bufferActions.openContent({
              type: 'diff',
              path: virtualPath,
              name: 'Uncommitted Changes',
              content: '',
              diffData: initialMultiDiff,
            }) ?? ''

          const repoPath = activeRepoPath
          const diffableFiles = visibleGitFiles.filter((entry) => entry.status !== 'untracked')
          const diffEntries = Array.from(
            new Map(
              diffableFiles.map((entry) => [
                `${entry.staged ? 'staged' : 'unstaged'}:${entry.path}`,
                entry,
              ]),
            ).entries(),
          ).filter(([fileKey]) => fileKey !== selectedFileKey)

          if (diffEntries.length > 0) {
            void (async () => {
              const accumulatedDiffs = [{ fileKey: selectedFileKey, diff }]

              for (const [fileKey, entry] of diffEntries) {
                const nextDiff = await getFileDiff(repoPath, entry.path, entry.staged)
                if (!nextDiff || (nextDiff.lines.length === 0 && nextDiff.is_image !== true)) {
                  continue
                }
                accumulatedDiffs.push({ fileKey, diff: nextDiff })

                const stats = countDiffStats(accumulatedDiffs.map((item) => item.diff))
                const partialDiff: MultiFileDiff = {
                  title: 'Uncommitted Changes',
                  commitHash: 'working-tree',
                  files: accumulatedDiffs.map((item) => item.diff),
                  totalFiles: accumulatedDiffs.length,
                  totalAdditions: stats.additions,
                  totalDeletions: stats.deletions,
                  fileKeys: accumulatedDiffs.map((item) => item.fileKey),
                  initiallyExpandedFileKey: selectedFileKey,
                  isLoading: true,
                }
                getActiveWorkspaceStoreRef()?.setState((state) => ({
                  ...state,
                  buffers: state.buffers.map((b) =>
                    b.id === bufferId && b.type === 'diff' ? { ...b, diffData: partialDiff } : b,
                  ),
                }))
                await Promise.resolve()
              }

              const allStats = countDiffStats(accumulatedDiffs.map((item) => item.diff))
              const finalDiff: MultiFileDiff = {
                title: 'Uncommitted Changes',
                commitHash: 'working-tree',
                files: accumulatedDiffs.map((item) => item.diff),
                totalFiles: accumulatedDiffs.length,
                totalAdditions: allStats.additions,
                totalDeletions: allStats.deletions,
                fileKeys: accumulatedDiffs.map((item) => item.fileKey),
                initiallyExpandedFileKey: selectedFileKey,
                isLoading: false,
              }
              getActiveWorkspaceStoreRef()?.setState((state) => ({
                ...state,
                buffers: state.buffers.map((b) =>
                  b.id === bufferId && b.type === 'diff' ? { ...b, diffData: finalDiff } : b,
                ),
              }))
            })()
          } else {
            const finalDiffData: MultiFileDiff = { ...initialMultiDiff, isLoading: false }
            getActiveWorkspaceStoreRef()?.setState((state) => ({
              ...state,
              buffers: state.buffers.map((b) =>
                b.id === bufferId && b.type === 'diff' ? { ...b, diffData: finalDiffData } : b,
              ),
            }))
          }
        } else {
          await handleOpenOriginalFile(actualFilePath)
        }
      } catch (error) {
        console.error('Error getting file diff:', error)
        await primitiveAlert(`Failed to get diff for ${filePath}:\n${error}`, 'Git Diff')
      }
    },
    [activeRepoPath, onFileSelect, visibleGitFiles, handleOpenOriginalFile],
  )

  const handleViewCommitDiff = useCallback(
    async (commitHash: string, filePath?: string) => {
      if (!activeRepoPath || !onFileSelect) return
      try {
        const diffs = await getCommitDiff(activeRepoPath, commitHash)
        const commit = useGitStore.getState().commits.find((entry) => entry.hash === commitHash)

        if (diffs && diffs.length > 0) {
          if (filePath) {
            const diff = diffs.find((d) => d.file_path === filePath) || diffs[0]
            const diffFileName = `${diff.file_path.split('/').pop()}.diff`
            getActiveWorkspaceStoreRef()
              ?.getState()
              .bufferActions.openContent({
                type: 'diff',
                path: `diff://commit/${commitHash}/${diffFileName}`,
                name: diffFileName,
                content: '',
                diffData: diff,
              })
          } else {
            const { additions, deletions } = countDiffStats(diffs)
            const multiDiff: MultiFileDiff = {
              title: `Commit ${commitHash.substring(0, 7)}`,
              repoPath: activeRepoPath,
              commitHash,
              commitMessage: commit?.message,
              commitDescription: commit?.description,
              commitAuthor: commit?.author,
              commitDate: commit?.date,
              files: diffs,
              totalFiles: diffs.length,
              totalAdditions: additions,
              totalDeletions: deletions,
            }
            getActiveWorkspaceStoreRef()
              ?.getState()
              .bufferActions.openContent({
                type: 'diff',
                path: `diff://commit/${commitHash}/all-files`,
                name: `Commit ${commitHash.substring(0, 7)} (${diffs.length} files)`,
                content: '',
                diffData: multiDiff,
              })
          }
        } else {
          await primitiveAlert(
            `No changes in this commit${filePath ? ` for file ${filePath}` : ''}.`,
            'Git Diff',
          )
        }
      } catch (error) {
        console.error('Error getting commit diff:', error)
        await primitiveAlert(`Failed to get diff for commit ${commitHash}:\n${error}`, 'Git Diff')
      }
    },
    [activeRepoPath, onFileSelect],
  )

  const handleViewStashDiff = useCallback(
    async (stashIndex: number) => {
      if (!activeRepoPath || !onFileSelect) return
      try {
        const diffs = await getStashDiff(activeRepoPath, stashIndex)
        if (diffs && diffs.length > 0) {
          const { additions, deletions } = countDiffStats(diffs)
          const multiDiff: MultiFileDiff = {
            repoPath: activeRepoPath,
            commitHash: `stash@{${stashIndex}}`,
            files: diffs,
            totalFiles: diffs.length,
            totalAdditions: additions,
            totalDeletions: deletions,
          }
          getActiveWorkspaceStoreRef()
            ?.getState()
            .bufferActions.openContent({
              type: 'diff',
              path: `diff://stash/${stashIndex}/all-files`,
              name: `Stash @{${stashIndex}} (${diffs.length} files)`,
              content: '',
              diffData: multiDiff,
            })
        } else {
          await primitiveAlert('No changes in this stash.', 'Git Diff')
        }
      } catch (error) {
        console.error('Error getting stash diff:', error)
        await primitiveAlert(`Failed to get diff for stash@{${stashIndex}}:\n${error}`, 'Git Diff')
      }
    },
    [activeRepoPath, onFileSelect],
  )

  const handleViewTagComparison = useCallback(
    async (baseRef: string, targetRef: string, title: string) => {
      if (!activeRepoPath || !onFileSelect) return
      try {
        const diffs = await getRefDiff(activeRepoPath, baseRef, targetRef)
        if (diffs && diffs.length > 0) {
          const { additions, deletions } = countDiffStats(diffs)
          const multiDiff: MultiFileDiff = {
            title,
            repoPath: activeRepoPath,
            commitHash: `${baseRef}..${targetRef}`,
            files: diffs,
            totalFiles: diffs.length,
            totalAdditions: additions,
            totalDeletions: deletions,
          }
          getActiveWorkspaceStoreRef()
            ?.getState()
            .bufferActions.openContent({
              type: 'diff',
              path: `diff://tag/${encodeURIComponent(title)}/all-files`,
              name: `${title} (${diffs.length} files)`,
              content: '',
              diffData: multiDiff,
            })
        } else {
          await primitiveAlert(`No changes between ${baseRef} and ${targetRef}.`, 'Git Diff')
        }
      } catch (error) {
        console.error('Error getting tag comparison:', error)
        await primitiveAlert(`Failed to compare ${baseRef} and ${targetRef}:\n${error}`, 'Git Diff')
      }
    },
    [activeRepoPath, onFileSelect],
  )

  return {
    handleOpenOriginalFile,
    handleViewFileDiff,
    handleViewCommitDiff,
    handleViewStashDiff,
    handleViewTagComparison,
  }
}
