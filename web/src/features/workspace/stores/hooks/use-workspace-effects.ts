import { useEffect } from 'react'
import { useFileSystemStore } from '@/features/file-system/controllers/store'
import { useBufferActions } from './use-buffer-store'
import { fileTreeQueryOptions, fileContentQueryOptions } from '@/lib/queries'
import { queryClient } from '@/lib/queries/client'
import type { AppFile } from '@/features/file-system/types/app'

export function useWorkspaceEffects(wsId: string, label?: string) {
  const bufferActions = useBufferActions()
  const repoPath = `/repos/${wsId}`

  // Seed file system from API
  useEffect(() => {
    queryClient.fetchQuery(fileTreeQueryOptions(repoPath))
      .then(files => {
        useFileSystemStore.setState({
          rootFolderPath: repoPath,
          files: files as unknown as AppFile[],
          handleFileOpen: async (path: string, revealOrIsDir?: boolean) => {
            if (revealOrIsDir === true) return
            const name = path.split('/').pop() ?? path
            const content = await queryClient.fetchQuery(fileContentQueryOptions(path))
            bufferActions.openContent({ type: 'editor', path, name, content })
          },
          handleFileSelect: (path: string, isDir?: boolean) => {
            if (isDir) return
            const name = path.split('/').pop() ?? path
            queryClient.fetchQuery(fileContentQueryOptions(path)).then(content => {
              bufferActions.openContent({ type: 'editor', path, name, content, isPreview: true })
            })
          },
        })
      })
      .catch(() => {})
  }, [repoPath]) // eslint-disable-line react-hooks/exhaustive-deps

  // Open crowbarChat buffer
  useEffect(() => {
    const name = label ?? 'Workspace'
    bufferActions.openContent({ type: 'crowbarChat', wsId, name })
  }, [wsId]) // eslint-disable-line react-hooks/exhaustive-deps

  // Open branchReview buffer
  useEffect(() => {
    const branchName = label ?? wsId
    bufferActions.openContent({ type: 'branchReview', wsId, branchName, name: branchName })
  }, [wsId]) // eslint-disable-line react-hooks/exhaustive-deps
}
