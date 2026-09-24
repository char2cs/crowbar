import ignore from 'ignore'
import { useCallback, useMemo } from 'react'
import {
  createFileTreeGitStatusLookup,
  getFileTreeEntryGitStatusDecoration,
  resolveActiveWorkspaceGitStatus,
  type FileTreeGitStatusDecoration,
} from '@/features/file-explorer/lib/file-tree-git-status'
import { collectGitIgnoreFileReferences } from '@/features/file-explorer/lib/file-tree-gitignore'
import type { FileEntry } from '@/features/file-system/types/app'
import { useGitStore } from '@/features/git/stores/git-store'
import { useSettingsStore } from '@/features/settings/store'
import { frontendTrace } from '@/utils/frontend-trace'
import { getRelativePath, pathStartsWithRoot } from '@/utils/path-helpers'
import { isAlwaysHiddenFileName, isHiddenFileTreeName } from '../lib/open-all'
import { useFileExplorerGitignore } from './use-file-explorer-gitignore'

const elapsedMs = (startedAt: number) => Math.round((performance.now() - startedAt) * 100) / 100

/**
 * The explorer's view of the workspace: the loaded tree with always-hidden,
 * user-hidden, dot and gitignored entries removed (per settings), plus the
 * git-status decoration for any entry. `isVisible` applies the same rules to a
 * path read straight from disk ("Open All" walks the directory itself).
 */
export function useFilteredFileTree({
  workspaceId,
  files,
  rootFolderPath,
}: {
  workspaceId: string | null
  files: FileEntry[]
  rootFolderPath?: string
}) {
  const hiddenFilePatterns = useSettingsStore((s) => s.settings.hiddenFilePatterns)
  const hiddenDirectoryPatterns = useSettingsStore((s) => s.settings.hiddenDirectoryPatterns)
  const showHidden = useSettingsStore((s) => s.settings.showHiddenFilesInFileTree)
  const showGitignored = useSettingsStore((s) => s.settings.showGitignoredFilesInFileTree)
  const showGitStatus = useSettingsStore((s) => s.settings.showGitStatusInFileTree)
  const workspaceGitStatus = useGitStore((state) => state.workspaceGitStatus)
  const currentWorkspaceRepoPath = useGitStore((state) => state.currentWorkspaceRepoPath)

  const userIgnore = useMemo(() => {
    const ig = ignore()
    if (hiddenFilePatterns.length > 0) ig.add(hiddenFilePatterns)
    if (hiddenDirectoryPatterns.length > 0) {
      ig.add(hiddenDirectoryPatterns.map((p) => (p.endsWith('/') ? p : `${p}/`)))
    }
    return ig
  }, [hiddenFilePatterns, hiddenDirectoryPatterns])

  const workspaceRootPaths = useMemo(() => {
    const roots: string[] = []
    for (const file of files) {
      if (file.isDir) roots.push(file.path)
    }
    if (rootFolderPath && !roots.includes(rootFolderPath)) roots.unshift(rootFolderPath)
    return roots
  }, [files, rootFolderPath])

  const getWorkspaceRootForPath = useCallback(
    (path: string) => workspaceRootPaths.find((rootPath) => pathStartsWithRoot(path, rootPath)),
    [workspaceRootPaths],
  )

  const isUserHidden = useCallback(
    (fullPath: string, isDir: boolean): boolean => {
      const matchedRootPath = getWorkspaceRootForPath(fullPath)
      if (!matchedRootPath) return false
      let relative = getRelativePath(fullPath, matchedRootPath)
      if (!relative || relative.trim() === '') return false
      if (isDir && !relative.endsWith('/')) relative += '/'
      return userIgnore.ignores(relative)
    },
    [getWorkspaceRootForPath, userIgnore],
  )

  const gitIgnoreFileReferences = useMemo(
    () => collectGitIgnoreFileReferences(files, rootFolderPath),
    [files, rootFolderPath],
  )
  const { isGitIgnored } = useFileExplorerGitignore(
    rootFolderPath,
    gitIgnoreFileReferences,
    getWorkspaceRootForPath,
  )

  const isVisible = useCallback(
    (path: string, name: string, isDir: boolean): boolean =>
      !isUserHidden(path, isDir) &&
      (showHidden || !isHiddenFileTreeName(name)) &&
      (showGitignored || !isGitIgnored(path, isDir)),
    [isUserHidden, isGitIgnored, showHidden, showGitignored],
  )

  const filteredFiles = useMemo(() => {
    const startedAt = performance.now()
    const process = (items: FileEntry[]): FileEntry[] =>
      items.flatMap((item) => {
        const isDir = item.isDir ?? false
        const ignored = isGitIgnored(item.path, isDir)
        if (isAlwaysHiddenFileName(item.name) || isUserHidden(item.path, isDir)) return []
        if (!showHidden && isHiddenFileTreeName(item.name)) return []
        if (!showGitignored && ignored) return []
        return [{ ...item, ignored, children: item.children ? process(item.children) : undefined }]
      })

    const result = process(files)
    frontendTrace('info', 'file-tree', 'filteredFiles:computed', {
      rootItems: files.length,
      filteredRootItems: result.length,
      durationMs: elapsedMs(startedAt),
    })
    return result
  }, [files, isGitIgnored, isUserHidden, showGitignored, showHidden])

  // The git store keys workspaceGitStatus by the wsId it loaded
  // (currentWorkspaceRepoPath). rootFolderPath is the synthetic `/repos/<repoId>`
  // mock-era prefix (a different id space), so it cannot be the match key.
  const gitStatus = resolveActiveWorkspaceGitStatus(
    workspaceGitStatus,
    currentWorkspaceRepoPath,
    workspaceId,
  )

  const gitStatusDecorationLookup = useMemo(() => {
    if (!gitStatus || !showGitStatus) return null
    const startedAt = performance.now()
    const lookup = createFileTreeGitStatusLookup(gitStatus)
    frontendTrace('info', 'file-tree', 'gitStatusDecorationLookup:computed', {
      gitFiles: gitStatus.files.length,
      filesMapSize: lookup.files.size,
      directoriesMapSize: lookup.directories.size,
      durationMs: elapsedMs(startedAt),
    })
    return lookup
  }, [gitStatus, showGitStatus])

  // Tree paths and git-status keys are both workspace-relative, so the lookup
  // matches them directly and returns null for files with no change.
  const getGitStatusDecoration = useCallback(
    (file: FileEntry): FileTreeGitStatusDecoration | null =>
      getFileTreeEntryGitStatusDecoration(file, rootFolderPath, gitStatusDecorationLookup),
    [gitStatusDecorationLookup, rootFolderPath],
  )

  return { filteredFiles, workspaceRootPaths, isVisible, getGitStatusDecoration }
}
