import { useCallback } from 'react'
import { primitiveAlert } from '@/components/ui/primitive-dialog-service'
import { getActiveWorkspaceStoreRef } from '@/features/workspace/stores/workspace-store-ref'
import { getCommitDiff, getRefDiff, getStashDiff } from '../api/git-diff-api'
import { useGitStore } from '../stores/git-store'
import type { MultiFileDiff } from '../types/git-diff-types'
import { countDiffStats } from '../utils/git-diff-helpers'

interface UseGitDiffHandlersProps {
  activeRepoPath: string | null
  onFileSelect?: (path: string, isDir: boolean) => void
}

export function useGitDiffHandlers({
  activeRepoPath,
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
    handleViewCommitDiff,
    handleViewStashDiff,
    handleViewTagComparison,
  }
}
