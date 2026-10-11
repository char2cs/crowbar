import { useCallback, useEffect, useState } from 'react'
import { readWorkspaceFile } from '@/features/file-system/controllers/platform'
import {
  createFileTreeGitIgnoreRules,
  isPathGitIgnoredByFileTreeRules,
  isWorkspaceRelativePath,
  type FileTreeGitIgnoreRules,
  type GitIgnoreFileContent,
  type GitIgnoreFileReference,
} from '@/features/file-explorer/lib/file-tree-gitignore'

interface UseFileExplorerGitignoreResult {
  isGitIgnored: (fullPath: string, isDir: boolean) => boolean
}

export function useFileExplorerGitignore(
  /** The workspace the tree shows: its ignore files are read from there. */
  workspaceId: string | null,
  rootFolderPath: string | undefined,
  gitIgnoreFileReferences: GitIgnoreFileReference[],
  getWorkspaceRootForPath: (path: string) => string | undefined,
): UseFileExplorerGitignoreResult {
  const [gitIgnoreRules, setGitIgnoreRules] = useState<FileTreeGitIgnoreRules | null>(null)

  useEffect(() => {
    let cancelled = false

    const loadGitignore = async () => {
      if (!rootFolderPath || !workspaceId) {
        setGitIgnoreRules(null)
        return
      }

      const ignoreFiles = await Promise.all(
        gitIgnoreFileReferences.map(async (file): Promise<GitIgnoreFileContent | null> => {
          try {
            return {
              ...file,
              content: await readWorkspaceFile(workspaceId, file.path),
            }
          } catch {
            return null
          }
        }),
      )

      if (!cancelled) {
        setGitIgnoreRules(
          createFileTreeGitIgnoreRules(
            rootFolderPath,
            ignoreFiles.filter((file): file is GitIgnoreFileContent => file !== null),
          ),
        )
      }
    }

    void loadGitignore()

    return () => {
      cancelled = true
    }
  }, [gitIgnoreFileReferences, rootFolderPath, workspaceId])

  const isGitIgnored = useCallback(
    (fullPath: string, isDir: boolean): boolean => {
      if (!gitIgnoreRules || !rootFolderPath) return false
      // Workspace-relative backend paths always belong to the active root; the
      // multi-root workspace lookup only applies to absolute desktop paths.
      if (
        !isWorkspaceRelativePath(fullPath) &&
        getWorkspaceRootForPath(fullPath) !== rootFolderPath
      ) {
        return false
      }
      return isPathGitIgnoredByFileTreeRules(gitIgnoreRules, fullPath, isDir)
    },
    [gitIgnoreRules, rootFolderPath, getWorkspaceRootForPath],
  )

  return { isGitIgnored }
}
