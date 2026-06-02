import { useEffect } from 'react'
import { useFileSystemStore } from '@/features/file-system/controllers/store'
import { useBufferActions } from './use-buffer-store'
import { useFileTreeStore } from '@/features/files/stores/file-tree-store'
import { dataOf } from '@/lib/loadable'
import { openFileContent } from '@/features/workspace/lib/open-file-content'
import type { AppFile } from '@/features/file-system/types/app'

export function useWorkspaceEffects(wsId: string) {
  const bufferActions = useBufferActions()
  const repoPath = `/repos/${wsId}`

  // Seed file system from API
  useEffect(() => {
    void (async () => {
      await useFileTreeStore.getState().fetch(repoPath)
      const files = dataOf(useFileTreeStore.getState().data)
      if (!files) return
      useFileSystemStore.setState({
        rootFolderPath: repoPath,
        files: files as unknown as AppFile[],
        handleFileOpen: async (path: string, revealOrIsDir?: boolean) => {
          if (revealOrIsDir === true) return
          await openFileContent(path, bufferActions, { preview: false })
        },
        handleFileSelect: (path: string, isDir?: boolean) => {
          if (isDir) return
          void openFileContent(path, bufferActions, { preview: true })
        },
      })
    })()
  }, [repoPath]) // eslint-disable-line react-hooks/exhaustive-deps
}
