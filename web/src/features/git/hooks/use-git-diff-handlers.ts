import { useCallback } from 'react'
import { primitiveAlert } from '@/components/ui/primitive-dialog-service'
import { getActiveWorkspaceStoreRef } from '@/features/workspace/stores/workspace-store-ref'

interface UseGitDiffHandlersProps {
  activeRepoPath: string | null
  onFileSelect?: (path: string, isDir: boolean) => void
}

/**
 * The actions the git panel can take on a commit.
 *
 * `handleViewCommitDiff` opens a tab carrying the SHA and nothing else. It used
 * to fetch the whole commit diff up front and hand the payload to the tab,
 * which meant a commit's every line crossed the wire and sat in a buffer before
 * anything was drawn. The tab now renders on the windowed review surface and
 * pulls each file's patch as the viewport reaches it, so opening a commit costs
 * the same whether it changed three lines or a million.
 *
 * The stash and tag-comparison handlers that used to live here were removed
 * with the Monaco diff stack: nothing called either of them.
 */
export function useGitDiffHandlers({ activeRepoPath, onFileSelect }: UseGitDiffHandlersProps) {
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
    async (commitHash: string) => {
      // activeRepoPath is the workspace id the windowed routes are scoped to.
      if (!activeRepoPath) return
      const store = getActiveWorkspaceStoreRef()
      if (!store) {
        await primitiveAlert('No active workspace to open the commit in.', 'Git Diff')
        return
      }
      store.getState().bufferActions.openContent({
        type: 'commitDiff',
        wsId: activeRepoPath,
        sha: commitHash,
        name: `Commit ${commitHash.substring(0, 7)}`,
      })
    },
    [activeRepoPath],
  )

  return { handleOpenOriginalFile, handleViewCommitDiff }
}
