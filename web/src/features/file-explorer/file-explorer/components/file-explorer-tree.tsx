import ignore from 'ignore'
import { Check, Eye, Funnel, GitBranch, MagnifyingGlass as Search } from '@phosphor-icons/react'
import type React from 'react'
import { memo, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useDebounce } from 'use-debounce'
import { useEventListener } from 'usehooks-ts'
import { useFileClipboardStore } from '@/features/file-explorer/stores/file-explorer-clipboard-store'
import { useFileTreeStore } from '@/features/file-explorer/stores/file-explorer-tree-store'
import {
  computeFileTreeSearchHits,
  filterFileTreeForFffHits,
  getGuideAncestorRows,
  getStickyAncestorRows,
} from '@/features/file-explorer/lib/visible-file-tree-rows'
import {
  createFileTreeGitStatusLookup,
  getFileTreeEntryGitStatusDecoration,
  resolveActiveWorkspaceGitStatus,
  type FileTreeGitStatusDecoration,
  type FileTreeGitStatusLookup,
} from '@/features/file-explorer/lib/file-tree-git-status'
import { collectGitIgnoreFileReferences } from '@/features/file-explorer/lib/file-tree-gitignore'
import { FILE_TREE_DENSITY_CONFIG } from '@/features/file-explorer/lib/file-tree-density'
import { fileOpenBenchmark } from '@/features/editor/utils/file-open-benchmark'
import { findFileInTree } from '@/features/file-system/controllers/file-tree-utils'
import { readDirectory } from '@/features/file-system/controllers/platform'
import {
  useFileSystemStore,
  workspaceFoldersSupported,
} from '@/features/file-system/controllers/store'
import type { FileEntry } from '@/features/file-system/types/app'
import { useGitStore } from '@/features/git/stores/git-store'
import { useSettingsStore } from '@/features/settings/store'
import { toast } from '@/features/window/stores/toast-store'
import { isWorkspaceLockedInSidebar, useSidebarStore } from '@/lib/store/sidebar'
import { getWorkspaceScope } from '@/lib/workspace-scope'
import { Button } from '@/components/ui/button'
import { Dropdown, type MenuItem } from '@/components/ui/dropdown'
import { Input } from '@/components/ui/input'
import { SidebarEmptyActionState, SidebarHeader } from '@/components/ui/sidebar'
import { cn } from '@/utils/cn'
import { frontendTrace } from '@/utils/frontend-trace'
import { getRelativePath, pathStartsWithRoot } from '@/utils/path-helpers'
import { useFileExplorerContextMenu } from '../hooks/use-file-explorer-context-menu'
import { useFileExplorerDragDrop } from '../hooks/use-file-explorer-drag-drop'
import { useFileExplorerGitignore } from '../hooks/use-file-explorer-gitignore'
import { useFileExplorerInlineEditing } from '../hooks/use-file-explorer-inline-editing'
import { useFileExplorerSync } from '../hooks/use-file-explorer-sync'
import { useFileExplorerVisibleRows } from '../hooks/use-file-explorer-visible-rows'
import { FileExplorerDialogs } from './file-explorer-dialogs'
import { FILE_TREE_BASE_INDENT, FileExplorerTreeItem } from './file-explorer-tree-item'
import type { FileTreeGuideTarget } from './file-explorer-tree-item'
import { FileExplorerIcon } from './file-explorer-icon'
import '../styles/file-explorer-tree.css'

const ALWAYS_HIDDEN_FILE_NAMES = new Set(['.ds_store'])

const isAlwaysHiddenFileName = (name: string): boolean =>
  ALWAYS_HIDDEN_FILE_NAMES.has(name.toLowerCase())

const isHiddenFileTreeName = (name: string): boolean => name.startsWith('.') && name.length > 1

const getPathBaseName = (path: string): string => {
  const trimmedPath = path.replace(/[\\/]+$/, '')
  if (!trimmedPath) return path
  const segments = trimmedPath.split(/[\\/]/)
  return segments[segments.length - 1] || path
}

interface FileExplorerTreeProps {
  files: FileEntry[]
  activePath?: string
  updateActivePath?: (path: string) => void
  rootFolderPath?: string
  onFileSelect: (path: string, isDir: boolean) => void | Promise<void>
  onFileOpen?: (path: string, isDir: boolean) => void | Promise<void>
  onCreateNewFileInDirectory: (
    directoryPath: string,
    fileName: string,
  ) => void | string | Promise<string | undefined>
  onCreateNewFolderInDirectory?: (directoryPath: string, folderName: string) => void
  onDeletePath?: (path: string, isDir: boolean) => void
  onGenerateImage?: (directoryPath: string) => void
  onUpdateFiles?: (files: FileEntry[]) => void
  onRenamePath?: (path: string, newName?: string) => void
  onDuplicatePath?: (path: string) => void
  onRefreshDirectory?: (path: string) => void
  onRevealInFinder?: (path: string) => void
  onUploadFile?: (directoryPath: string) => void
  onFileMove?: (oldPath: string, newPath: string) => void
  filter?: 'all' | 'changed'
}

interface FileExplorerAlertDialogState {
  title: string
  message: string
}

interface OpenAllFilesDialogState {
  filePaths: string[]
}

const FILE_TREE_CONTAINER_INSET = 4
const FILE_TREE_HEADER_HEIGHT = 32
const FILE_TREE_SEARCH_DEBOUNCE_DELAY = 80
const getFileTreeRowId = (path: string) => `file-tree-row-${path.replace(/[^a-zA-Z0-9_-]/g, '_')}`

const handleRootDrop = (e: React.DragEvent) => {
  e.preventDefault()
  e.stopPropagation()
}

// react-doctor-disable-next-line no-giant-component -- accepted: cohesive virtualized tree — keyboard nav, search, drag-drop and gitignore all operate over one shared row model + refs; this is the tree's core, splitting hurts locality.
function FileExplorerTreeComponent({
  files,
  activePath,
  updateActivePath,
  rootFolderPath,
  onFileSelect,
  onFileOpen,
  onCreateNewFileInDirectory,
  onCreateNewFolderInDirectory,
  onDeletePath,
  onGenerateImage,
  onUpdateFiles,
  onRenamePath,
  onDuplicatePath,
  onRefreshDirectory,
  onRevealInFinder,
  onUploadFile,
  onFileMove,
  filter = 'all' as const,
}: FileExplorerTreeProps) {
  const [deleteCandidate, setDeleteCandidate] = useState<{
    path: string
    isDir: boolean
  } | null>(null)
  const [alertDialog, setAlertDialog] = useState<FileExplorerAlertDialogState | null>(null)
  const [openAllFilesDialog, setOpenAllFilesDialog] = useState<OpenAllFilesDialogState | null>(null)
  const [isDeletingPath, setIsDeletingPath] = useState(false)
  const [isOpeningAllFiles, setIsOpeningAllFiles] = useState(false)
  const [focusedPath, setFocusedPath] = useState<string | undefined>(activePath)
  const [hasTreeFocus, setHasTreeFocus] = useState(false)
  const [treeSearchOpen, setTreeSearchOpen] = useState(false)
  const [treeSearchQuery, setTreeSearchQuery] = useState('')
  const [debouncedTreeSearchQuery] = useDebounce(treeSearchQuery, FILE_TREE_SEARCH_DEBOUNCE_DELAY)
  const [isFileTreeFilterMenuOpen, setIsFileTreeFilterMenuOpen] = useState(false)
  const containerRef = useRef<HTMLDivElement>(null)
  const searchInputRef = useRef<HTMLInputElement>(null)
  const filterButtonRef = useRef<HTMLButtonElement>(null)
  const documentRef = useRef<Document>(document)
  const savedExpandedPathsRef = useRef<Set<string> | null>(null)

  const workspaceGitStatus = useGitStore((state) => state.workspaceGitStatus)
  const currentWorkspaceRepoPath = useGitStore((state) => state.currentWorkspaceRepoPath)
  // sticky handled purely by CSS; no JS scanning

  const settings = useSettingsStore((s) => s.settings)
  const updateSetting = useSettingsStore((s) => s.updateSetting)
  const fileTreeDensity = settings.fileTreeDensity
  const handleOpenFolder = useFileSystemStore((state) => state.handleOpenFolder)
  const addFolderToWorkspace = useFileSystemStore((state) => state.addFolderToWorkspace)
  const removeFolderFromWorkspace = useFileSystemStore((state) => state.removeFolderFromWorkspace)
  const revealPathInTree = useFileSystemStore((state) => state.revealPathInTree)
  const isFileTreeLoading = useFileSystemStore((state) => state.isFileTreeLoading)

  const handleAutoExpandDirectory = useCallback(
    (path: string) => {
      if (useFileTreeStore.getState().isExpanded(path)) return
      void Promise.resolve(onFileSelect(path, true))
    },
    [onFileSelect],
  )

  const showAlertDialog = useCallback((title: string, message: string) => {
    setAlertDialog({ title, message })
  }, [])

  const handleMoveError = useCallback(
    (message: string) => showAlertDialog('Move Failed', message),
    [showAlertDialog],
  )

  const { dragState, startDrag } = useFileExplorerDragDrop(
    rootFolderPath,
    onFileMove,
    handleAutoExpandDirectory,
    handleMoveError,
  )

  // Drag-threshold bookkeeping only — never rendered, so a ref avoids a
  // redraw on every mousedown/mousemove/mouseup during drag detection.
  const mouseDownInfoRef = useRef<{
    x: number
    y: number
    file: FileEntry
  } | null>(null)

  const userIgnore = useMemo(() => {
    const ig = ignore()
    if (settings.hiddenFilePatterns.length > 0) {
      ig.add(settings.hiddenFilePatterns)
    }
    if (settings.hiddenDirectoryPatterns.length > 0) {
      ig.add(settings.hiddenDirectoryPatterns.map((p) => (p.endsWith('/') ? p : `${p}/`)))
    }
    return ig
  }, [settings.hiddenFilePatterns, settings.hiddenDirectoryPatterns])

  const workspaceRootPaths = useMemo(() => {
    const roots: string[] = []
    for (const file of files) {
      if (file.isDir) roots.push(file.path)
    }
    if (rootFolderPath && !roots.includes(rootFolderPath)) {
      roots.unshift(rootFolderPath)
    }
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

  // removed scroll-time DOM scanning for sticky folders

  const gitIgnoreFileReferences = useMemo(
    () => collectGitIgnoreFileReferences(files, rootFolderPath),
    [files, rootFolderPath],
  )

  const { isGitIgnored } = useFileExplorerGitignore(
    rootFolderPath,
    gitIgnoreFileReferences,
    getWorkspaceRootForPath,
  )

  // The git store keys workspaceGitStatus by the wsId it loaded
  // (currentWorkspaceRepoPath). rootFolderPath is the synthetic `/repos/<repoId>`
  // mock-era prefix (a different id space), so it cannot be the match key — the
  // file explorer always renders the active workspace, so gate on its wsId.
  const activeWorkspaceId = getWorkspaceScope()?.wsId ?? null
  const gitStatus = resolveActiveWorkspaceGitStatus(
    workspaceGitStatus,
    currentWorkspaceRepoPath,
    activeWorkspaceId,
  )

  // A locked (protected-branch) worktree refuses every daemon write (409
  // "workspace locked"), so the context menu must not offer mutations there.
  // isWorkspaceLockedInSidebar checks tree rows AND the default (main-worktree)
  // workspace, which is not a tree row (repo.defaultWorkspaceId/…Status) —
  // adopted protected branches are locked defaults, so repo homes gate too.
  // Returns a boolean, so the selector stays referentially stable.
  const isWorkspaceLocked = useSidebarStore((s) =>
    isWorkspaceLockedInSidebar(s.repos, activeWorkspaceId),
  )

  const gitStatusDecorationLookup = useMemo(() => {
    const startedAt = performance.now()
    if (!gitStatus || !settings.showGitStatusInFileTree)
      return null as FileTreeGitStatusLookup | null

    const lookup = createFileTreeGitStatusLookup(gitStatus)

    frontendTrace('info', 'file-tree', 'gitStatusDecorationLookup:computed', {
      gitFiles: gitStatus.files.length,
      filesMapSize: lookup.files.size,
      directoriesMapSize: lookup.directories.size,
      durationMs: Math.round((performance.now() - startedAt) * 100) / 100,
    })
    return lookup
  }, [gitStatus, settings.showGitStatusInFileTree])

  // Resolve straight from the lookup: tree paths and git-status keys are both
  // workspace-relative, so getFileTreeEntryGitStatusDecoration matches them and
  // returns null for files with no change. (The old `getWorkspaceRootForPath ===
  // rootFolderPath` guard compared a relative path against the synthetic
  // `/repos/<repoId>` root — always false — and blocked every decoration.)
  const getGitStatusDecoration = useCallback(
    (file: FileEntry): FileTreeGitStatusDecoration | null =>
      getFileTreeEntryGitStatusDecoration(file, rootFolderPath, gitStatusDecorationLookup),
    [gitStatusDecorationLookup, rootFolderPath],
  )

  const filteredFiles = useMemo(() => {
    const startedAt = performance.now()
    const process = (items: FileEntry[]): FileEntry[] =>
      items.flatMap((item) => {
        const ignored = isGitIgnored(item.path, item.isDir ?? false)

        if (isAlwaysHiddenFileName(item.name) || isUserHidden(item.path, item.isDir ?? false)) {
          return []
        }

        if (!settings.showHiddenFilesInFileTree && isHiddenFileTreeName(item.name)) {
          return []
        }

        if (!settings.showGitignoredFilesInFileTree && ignored) {
          return []
        }

        return [
          {
            ...item,
            ignored,
            children: item.children ? process(item.children) : undefined,
          },
        ]
      })

    const result = process(files)
    frontendTrace('info', 'file-tree', 'filteredFiles:computed', {
      rootItems: files.length,
      filteredRootItems: result.length,
      durationMs: Math.round((performance.now() - startedAt) * 100) / 100,
    })
    return result
  }, [
    files,
    isGitIgnored,
    isUserHidden,
    settings.showGitignoredFilesInFileTree,
    settings.showHiddenFilesInFileTree,
  ])

  useFileExplorerSync({
    activePath,
    updateActivePath,
    revealPathInTree,
  })

  const isTreeSearchActive = treeSearchQuery.trim().length > 0
  const isTreeSearchSettling =
    isTreeSearchActive && treeSearchQuery.trim() !== debouncedTreeSearchQuery.trim()
  const isTreeSearchSearching = isTreeSearchActive && isTreeSearchSettling
  const treeSearchResult = useMemo(
    () =>
      filterFileTreeForFffHits(
        filteredFiles,
        computeFileTreeSearchHits(filteredFiles, debouncedTreeSearchQuery),
      ),
    [filteredFiles, debouncedTreeSearchQuery],
  )
  const displayedFiles = useMemo(
    () =>
      isTreeSearchActive && !isTreeSearchSearching
        ? treeSearchResult.files
        : isTreeSearchActive
          ? []
          : filteredFiles,
    [isTreeSearchActive, isTreeSearchSearching, treeSearchResult.files, filteredFiles],
  )
  const displayedExpandedPaths =
    isTreeSearchActive && !isTreeSearchSearching ? treeSearchResult.expandedPaths : undefined
  const hasActiveFileTreeFilters =
    !settings.showHiddenFilesInFileTree ||
    !settings.showGitignoredFilesInFileTree ||
    !settings.showGitStatusInFileTree
  const fileTreeFilterMenuItems = useMemo<MenuItem[]>(
    () => [
      {
        id: 'hidden-files',
        label: 'Hidden Files',
        icon: <Eye />,
        keybinding: settings.showHiddenFilesInFileTree ? (
          <Check className="size-3.5 text-primary" />
        ) : null,
        onClick: () =>
          void updateSetting('showHiddenFilesInFileTree', !settings.showHiddenFilesInFileTree),
      },
      {
        id: 'gitignored-files',
        label: 'Gitignored Files',
        icon: <GitBranch />,
        keybinding: settings.showGitignoredFilesInFileTree ? (
          <Check className="size-3.5 text-primary" />
        ) : null,
        onClick: () =>
          void updateSetting(
            'showGitignoredFilesInFileTree',
            !settings.showGitignoredFilesInFileTree,
          ),
      },
      { id: 'sep-status', label: '', separator: true, onClick: () => {} },
      {
        id: 'git-status',
        label: 'Git Status',
        icon: <GitBranch />,
        keybinding: settings.showGitStatusInFileTree ? (
          <Check className="size-3.5 text-primary" />
        ) : null,
        onClick: () =>
          void updateSetting('showGitStatusInFileTree', !settings.showGitStatusInFileTree),
      },
    ],
    [
      settings.showGitStatusInFileTree,
      settings.showGitignoredFilesInFileTree,
      settings.showHiddenFilesInFileTree,
      updateSetting,
    ],
  )

  const changedFilteredFiles = useMemo(() => {
    if (filter !== 'changed') return displayedFiles

    const pruneTree = (items: FileEntry[]): FileEntry[] =>
      items.flatMap((item) => {
        if (!item.isDir) {
          return getGitStatusDecoration(item) !== null ? [item] : []
        }
        const prunedChildren = pruneTree(item.children ?? [])
        const dirDecoration = getGitStatusDecoration(item)
        if (prunedChildren.length === 0 && dirDecoration === null) return []
        return [{ ...item, children: prunedChildren }]
      })

    return pruneTree(displayedFiles)
  }, [filter, displayedFiles, getGitStatusDecoration])

  const { visibleRows, rowVirtualizer } = useFileExplorerVisibleRows({
    files: changedFilteredFiles,
    activePath,
    containerRef,
    expandedPathsOverride: displayedExpandedPaths,
  })

  // Pre-compute guide targets for all rows when the row structure changes.
  // Calling getGuideAncestorRows per-row inside the render loop is O(N×depth) on
  // every scroll-triggered re-render; this memo moves that work to structural changes only.
  const guideTargetsByIndex = useMemo(() => {
    return visibleRows.map((_, i) =>
      getGuideAncestorRows(visibleRows, i).map((ancestor) =>
        ancestor
          ? {
              path: ancestor.file.path,
              name: ancestor.displayName ?? ancestor.file.name,
              isDir: ancestor.file.isDir ?? false,
              isActive: activePath
                ? activePath === ancestor.file.path ||
                  activePath.startsWith(`${ancestor.file.path}/`) ||
                  activePath.startsWith(`${ancestor.file.path}\\`)
                : false,
            }
          : null,
      ),
    )
  }, [visibleRows, activePath])

  const keyboardPath = focusedPath || activePath
  const highlightedPath = hasTreeFocus ? keyboardPath : activePath

  useEffect(() => {
    if (!hasTreeFocus) {
      // react-doctor-disable-next-line no-chain-state-updates -- FP: this is a one-way sync of the keyboard cursor to `activePath` while the tree is unfocused (so re-focus resumes at the active file). Nothing chains off `focusedPath` via another effect — it's only read at render (`keyboardPath = focusedPath || activePath`) and written by keyboard handlers — so there is no multi-step render cascade.
      setFocusedPath(activePath)
    }
  }, [activePath, hasTreeFocus])

  useEffect(() => {
    if (!treeSearchOpen) return
    const rafId = requestAnimationFrame(() => {
      searchInputRef.current?.focus()
      searchInputRef.current?.select()
    })

    return () => cancelAnimationFrame(rafId)
  }, [treeSearchOpen])

  const closeTreeSearch = useCallback(() => {
    setTreeSearchOpen(false)
    setTreeSearchQuery('')
    containerRef.current?.focus()
  }, [])

  const navigateTreeSearchMatch = useCallback(
    (direction: 1 | -1) => {
      if (!isTreeSearchActive || treeSearchResult.matchedPaths.size === 0) return

      // visibleRows can be large (every visible row in the tree), so index it
      // once by path instead of calling findIndex per matched path.
      const rowIndexByPath = new Map(visibleRows.map((row, index) => [row.file.path, index]))
      const matchIndexes: number[] = []
      for (const path of treeSearchResult.orderedMatchedPaths) {
        const index = rowIndexByPath.get(path)
        if (index !== undefined) matchIndexes.push(index)
      }

      if (matchIndexes.length === 0) return

      const currentIndex = keyboardPath ? (rowIndexByPath.get(keyboardPath) ?? -1) : -1
      const fallbackIndex = direction > 0 ? matchIndexes[0] : matchIndexes[matchIndexes.length - 1]
      const nextIndex =
        direction > 0
          ? (matchIndexes.find((index) => index > currentIndex) ?? fallbackIndex)
          : ([...matchIndexes].reverse().find((index) => index < currentIndex) ?? fallbackIndex)
      const nextPath = visibleRows[nextIndex]?.file.path

      if (nextPath) {
        setFocusedPath(nextPath)
        rowVirtualizer.scrollToIndex(nextIndex, { align: 'auto' })
      }
    },
    [
      isTreeSearchActive,
      keyboardPath,
      rowVirtualizer,
      treeSearchResult.matchedPaths,
      treeSearchResult.orderedMatchedPaths,
      visibleRows,
    ],
  )

  useEffect(() => {
    if (!isTreeSearchActive || treeSearchResult.matchedPaths.size === 0) return
    if (keyboardPath && treeSearchResult.matchedPaths.has(keyboardPath)) return

    const firstMatchIndex =
      treeSearchResult.orderedMatchedPaths
        .map((path) => visibleRows.findIndex((row) => row.file.path === path))
        .find((index) => index >= 0) ?? -1
    const firstMatchPath = visibleRows[firstMatchIndex]?.file.path

    if (!firstMatchPath) return

    setFocusedPath(firstMatchPath)
    rowVirtualizer.scrollToIndex(firstMatchIndex, { align: 'auto' })
  }, [
    isTreeSearchActive,
    keyboardPath,
    rowVirtualizer,
    treeSearchResult.matchedPaths,
    treeSearchResult.orderedMatchedPaths,
    visibleRows,
  ])

  useEffect(() => {
    const handleFileTreeOpenSearch = () => {
      setTreeSearchOpen(true)
    }

    window.addEventListener('file-tree-open-search', handleFileTreeOpenSearch)
    return () => window.removeEventListener('file-tree-open-search', handleFileTreeOpenSearch)
  }, [])

  // When search is active, expand all directories so the lazy loader fetches their
  // children — otherwise files in unexpanded dirs are invisible to the search.
  // The pre-search expansion state is saved and restored when search clears.
  useEffect(() => {
    if (debouncedTreeSearchQuery.trim()) {
      if (!savedExpandedPathsRef.current) {
        savedExpandedPathsRef.current = new Set(useFileTreeStore.getState().expandedPaths)
      }
      useFileTreeStore.getState().expandAll(filteredFiles)
    } else {
      if (savedExpandedPathsRef.current) {
        useFileTreeStore.getState().setExpandedPaths(savedExpandedPathsRef.current)
        savedExpandedPathsRef.current = null
      }
    }
  }, [debouncedTreeSearchQuery, filteredFiles])

  // No sticky overlays or global guides

  const { editingValue, setEditingValue, startInlineEditing, handleKeyDown, handleBlur } =
    useFileExplorerInlineEditing({
      files,
      rootFolderPath,
      onUpdateFiles,
      onRenamePath,
      onCreateNewFileInDirectory,
      onCreateNewFolderInDirectory,
      showAlertDialog,
    })

  const openPathInTab = useCallback(
    async (path: string) => {
      if (onFileOpen) {
        await Promise.resolve(onFileOpen(path, false))
        return
      }
      await Promise.resolve(onFileSelect(path, false))
    },
    [onFileOpen, onFileSelect],
  )

  const collectLoadedFilesInDirectory = useCallback(
    (directoryPath: string): string[] => {
      // The workspace root is addressed by its absolute path (=== rootFolderPath)
      // or '', but the tree's own nodes are root-relative, so there is no node to
      // look up — walk the top-level entries directly. Any other directory is a
      // real relative-path node in the tree.
      const isRoot = !directoryPath || directoryPath === rootFolderPath
      let rootEntries: FileEntry[] | undefined
      if (isRoot) {
        rootEntries = filteredFiles
      } else {
        const directory = findFileInTree(filteredFiles, directoryPath)
        if (!directory || !directory.isDir) return []
        rootEntries = directory.children
      }

      const collected: string[] = []
      const walk = (entries?: FileEntry[]) => {
        if (!entries) return
        for (const entry of entries) {
          if (entry.isDir) {
            walk(entry.children)
          } else {
            collected.push(entry.path)
          }
        }
      }

      walk(rootEntries)
      return collected
    },
    [filteredFiles, rootFolderPath],
  )

  const collectLocalFilesInDirectory = useCallback(
    async (directoryPath: string): Promise<string[]> => {
      const collected: string[] = []
      const stack: string[] = [directoryPath]

      while (stack.length > 0) {
        const currentPath = stack.pop()
        if (!currentPath) continue

        const entries = await readDirectory(currentPath)
        for (const entry of entries as Array<{
          path: string
          is_dir?: boolean
        }>) {
          if (!entry.path) continue
          const isDir = !!entry.is_dir
          const entryName = getPathBaseName(entry.path)

          if (isAlwaysHiddenFileName(entryName)) {
            continue
          }

          if (isUserHidden(entry.path, isDir)) {
            continue
          }

          if (!settings.showHiddenFilesInFileTree && isHiddenFileTreeName(entryName)) {
            continue
          }

          if (!settings.showGitignoredFilesInFileTree && isGitIgnored(entry.path, isDir)) {
            continue
          }

          if (isDir) {
            stack.push(entry.path)
          } else {
            collected.push(entry.path)
          }
        }
      }

      return collected
    },
    [
      isUserHidden,
      isGitIgnored,
      settings.showGitignoredFilesInFileTree,
      settings.showHiddenFilesInFileTree,
    ],
  )

  const openFilePathsInTabs = useCallback(
    async (filePaths: string[]) => {
      for (const filePath of filePaths) {
        // react-doctor-disable-next-line async-await-in-loop -- kept sequential: each open reads the pane's current tab list via getState() and appends, so concurrent opens could race on that read-modify-write and land tabs out of drop order. Rare (multi-file drag-drop), not a hot path.
        await openPathInTab(filePath)
      }

      updateActivePath?.(filePaths[filePaths.length - 1])
    },
    [openPathInTab, updateActivePath],
  )

  const handleOpenAllFilesInDirectory = useCallback(
    async (directoryPath: string) => {
      let filePaths: string[]

      if (directoryPath.startsWith('remote://')) {
        filePaths = collectLoadedFilesInDirectory(directoryPath)
      } else {
        try {
          filePaths = await collectLocalFilesInDirectory(directoryPath)
        } catch (error) {
          console.error(
            'Failed to scan directory for Open All, falling back to loaded tree:',
            error,
          )
          filePaths = collectLoadedFilesInDirectory(directoryPath)
        }
      }

      const uniqueFilePaths = Array.from(new Set(filePaths))
      if (uniqueFilePaths.length === 0) return

      if (uniqueFilePaths.length > 100) {
        setOpenAllFilesDialog({ filePaths: uniqueFilePaths })
        return
      }

      await openFilePathsInTabs(uniqueFilePaths)
    },
    [collectLoadedFilesInDirectory, collectLocalFilesInDirectory, openFilePathsInTabs],
  )

  const handleOpenAllFilesConfirm = useCallback(async () => {
    if (!openAllFilesDialog) return

    setIsOpeningAllFiles(true)
    try {
      await openFilePathsInTabs(openAllFilesDialog.filePaths)
      setOpenAllFilesDialog(null)
    } finally {
      setIsOpeningAllFiles(false)
    }
  }, [openAllFilesDialog, openFilePathsInTabs])

  const { setContextMenu, handleContextMenu, contextMenuElement, fileFeedback } =
    useFileExplorerContextMenu({
      rootFolderPath,
      // Locked worktrees refuse writes — hide every mutation item (New File/Folder,
      // Upload, Duplicate, Rename, Delete, Cut, env-template).
      isLocked: isWorkspaceLocked,
      onFileSelect,
      onCreateNewFileInDirectory,
      onGenerateImage,
      onRefreshDirectory,
      onRenamePath,
      onRevealInFinder,
      onUploadFile,
      onDuplicatePath,
      // The store impls behind these are still no-op stubs (no multi-root
      // workspace model exists yet); an undefined handler hides the menu items
      // so no dead actions render. See workspaceFoldersSupported (Task 28).
      onAddFolderToWorkspace: workspaceFoldersSupported
        ? () => {
            void addFolderToWorkspace()
          }
        : undefined,
      onRemoveFolderFromWorkspace: workspaceFoldersSupported
        ? (path) => {
            void removeFolderFromWorkspace(path)
          }
        : undefined,
      // Only the actual workspace root hides Rename/Delete. workspaceRootPaths
      // lists every top-level folder (a multi-root-workspace notion that doesn't
      // apply to Crowbar's single worktree); using it here wrongly treated every
      // top-level folder as a root, so Delete/Rename never appeared on them.
      isWorkspaceRootPath: (path) => path === rootFolderPath,
      canRemoveWorkspaceRootPath: (path) =>
        path !== rootFolderPath && workspaceRootPaths.includes(path),
      onDeleteRequested: setDeleteCandidate,
      onStartInlineEditing: startInlineEditing,
      onOpenAllFilesInDirectory: handleOpenAllFilesInDirectory,
    })

  useEventListener(
    'keydown',
    (e: KeyboardEvent) => {
      if (e.key === 'Escape') setContextMenu(null)
    },
    documentRef,
  )

  useEventListener('dragover', (e: DragEvent) => e.preventDefault(), documentRef)

  // Fast path->file lookup for delegation
  const pathToFile = useMemo(() => {
    const m = new Map<string, FileEntry>()
    for (const r of visibleRows) m.set(r.file.path, r.file)
    return m
  }, [visibleRows])

  const getTargetItem = useCallback(
    (target: EventTarget | null) => {
      const el = (target as HTMLElement | null)?.closest('[data-file-path]') as
        (HTMLElement & { dataset: { filePath?: string; isDir?: string } }) | null
      if (!el) return null
      const path = el.dataset.filePath || el.getAttribute('data-file-path') || ''
      const isDir = (el.dataset.isDir || el.getAttribute('data-is-dir')) === 'true'
      const file = pathToFile.get(path)
      if (!file) return null
      return { path, isDir, file }
    },
    [pathToFile],
  )

  const toggleDirectory = useCallback(
    async (path: string) => {
      await Promise.resolve(onFileSelect(path, true))
    },
    [onFileSelect],
  )

  const handleContainerClick = useCallback(
    (e: React.MouseEvent) => {
      const t = getTargetItem(e.target)
      if (!t) {
        e.preventDefault()
        e.stopPropagation()
        setFocusedPath(undefined)
        updateActivePath?.('')
        return
      }
      e.preventDefault()
      e.stopPropagation()
      if (!t.isDir) {
        fileOpenBenchmark.ensureStarted(t.path, 'explorer-click')
        fileOpenBenchmark.mark(t.path, 'explorer-click')
      }
      if (t.isDir) {
        void toggleDirectory(t.path)
        setFocusedPath(t.path)
        updateActivePath?.(t.path)
      } else {
        setFocusedPath(t.path)
        void Promise.resolve(onFileSelect(t.path, false))
      }
    },
    [onFileSelect, toggleDirectory, updateActivePath, getTargetItem],
  )

  const handleContainerDoubleClick = useCallback(
    (e: React.MouseEvent) => {
      const t = getTargetItem(e.target)
      if (!t) return
      e.preventDefault()
      e.stopPropagation()
      setFocusedPath(t.path)
      // Double-click begins an inline rename (onRenamePath with no new name marks
      // the node editable). Single-click still opens/previews; the context-menu
      // "Rename" uses this same path.
      onRenamePath?.(t.path)
    },
    [onRenamePath, getTargetItem],
  )

  const handleContainerContextMenu = useCallback(
    (e: React.MouseEvent) => {
      const t = getTargetItem(e.target)
      if (t) {
        handleContextMenu(e, t.path, t.isDir)
        return
      }

      if (rootFolderPath) {
        handleContextMenu(e, rootFolderPath, true)
      }
    },
    [handleContextMenu, rootFolderPath, getTargetItem],
  )

  const handleContainerMouseDown = useCallback(
    (e: React.MouseEvent) => {
      if (e.button !== 0) return
      const t = getTargetItem(e.target)
      if (!t) return
      mouseDownInfoRef.current = { x: e.clientX, y: e.clientY, file: t.file }
    },
    [getTargetItem],
  )

  const handleContainerMouseMove = useCallback(
    (e: React.MouseEvent) => {
      const info = mouseDownInfoRef.current
      if (info && !dragState.isDragging) {
        const dx = e.clientX - info.x
        const dy = e.clientY - info.y
        if (Math.hypot(dx, dy) > 5) {
          startDrag(e, info.file)
          mouseDownInfoRef.current = null
        }
      }
    },
    [dragState.isDragging, startDrag],
  )

  const handleContainerMouseUp = useCallback(() => {
    mouseDownInfoRef.current = null
  }, [])
  const handleContainerMouseLeave = useCallback(() => {
    mouseDownInfoRef.current = null
  }, [])

  // No recursive render; rows are virtualized

  const handleDeleteConfirm = useCallback(async () => {
    if (!deleteCandidate) return

    setIsDeletingPath(true)
    try {
      await Promise.resolve(onDeletePath?.(deleteCandidate.path, deleteCandidate.isDir))
      setDeleteCandidate(null)
    } finally {
      setIsDeletingPath(false)
    }
  }, [deleteCandidate, onDeletePath])

  useEffect(() => {
    if (!activePath || !fileOpenBenchmark.has(activePath)) return

    fileOpenBenchmark.mark(activePath, 'explorer-active-path')

    const rafId = requestAnimationFrame(() => {
      fileOpenBenchmark.mark(activePath, 'explorer-painted')
    })

    return () => cancelAnimationFrame(rafId)
  }, [activePath])

  return (
    <div
      className={cn(
        'file-tree-container relative flex min-w-full flex-1 select-none flex-col overflow-auto p-0',
        dragState.dragOverPath === '__ROOT__' &&
          'border-2! border-dashed! border-secondary! bg-secondary! bg-opacity-10!',
      )}
      ref={containerRef}
      style={{ scrollBehavior: 'auto', overscrollBehavior: 'contain' }}
      role="tree"
      aria-label="File Explorer"
      aria-activedescendant={highlightedPath ? getFileTreeRowId(highlightedPath) : undefined}
      tabIndex={0}
      onFocusCapture={() => {
        setHasTreeFocus(true)
        setFocusedPath((current) => current || activePath || visibleRows[0]?.file.path)
      }}
      onBlurCapture={(e) => {
        if (!e.currentTarget.contains(e.relatedTarget as Node | null)) {
          setHasTreeFocus(false)
        }
      }}
      onKeyDown={(e) => {
        const mod = e.metaKey || e.ctrlKey
        if (mod && e.key.toLowerCase() === 'f') {
          e.preventDefault()
          e.stopPropagation()
          setTreeSearchOpen(true)
          return
        }

        if (!mod && !e.altKey && !e.shiftKey && e.key === '/') {
          e.preventDefault()
          e.stopPropagation()
          setTreeSearchOpen(true)
          return
        }

        // Let inputs handle their own keys
        const tag = (e.target as HTMLElement).tagName
        if (tag === 'INPUT' || tag === 'TEXTAREA' || (e.target as HTMLElement).isContentEditable) {
          return
        }
        const index = visibleRows.findIndex((r) => r.file.path === keyboardPath)
        const curIndex = index === -1 ? 0 : index
        const current = visibleRows[curIndex]?.file
        const isDir = visibleRows[curIndex]?.file.isDir

        const clipboardActions = useFileClipboardStore.getState().actions
        if (mod && current) {
          if (e.key === 'c') {
            e.preventDefault()
            clipboardActions.copy([{ path: current.path, is_dir: !!isDir }])
            return
          }
          if (e.key === 'x') {
            e.preventDefault()
            clipboardActions.cut([{ path: current.path, is_dir: !!isDir }])
            return
          }
          if (e.key === 'v') {
            e.preventDefault()
            const sep = current.path.includes('\\') ? '\\' : '/'
            const targetDir = isDir ? current.path : current.path.split(sep).slice(0, -1).join(sep)
            if (targetDir) {
              // Paste really moves/copies now, so it can really fail (an
              // occupied destination is a 409 on both verbs — the daemon never
              // clobbers). Say so instead of leaving the user with a silently
              // unfinished move, and never leave the rejection unhandled.
              clipboardActions
                .paste(targetDir)
                .then((results) => {
                  onRefreshDirectory?.(targetDir)
                  const failed = results.filter((result) => !result.success)
                  if (failed.length > 0) {
                    toast.error(
                      'Paste failed',
                      failed.map((result) => result.destination_path).join(', '),
                    )
                  }
                })
                .catch((error: unknown) => {
                  toast.error(
                    'Paste failed',
                    error instanceof Error ? error.message : String(error),
                  )
                })
            }
            return
          }
        }

        switch (e.key) {
          case 'Escape': {
            e.preventDefault()
            e.stopPropagation()
            setContextMenu(null)
            containerRef.current?.focus()
            break
          }
          case 'ArrowDown': {
            e.preventDefault()
            const next = Math.min(visibleRows.length - 1, curIndex + 1)
            const p = visibleRows[next]?.file.path
            if (p) {
              setFocusedPath(p)
              rowVirtualizer.scrollToIndex(next)
            }
            break
          }
          case 'ArrowUp': {
            e.preventDefault()
            const prev = Math.max(0, curIndex - 1)
            const p = visibleRows[prev]?.file.path
            if (p) {
              setFocusedPath(p)
              rowVirtualizer.scrollToIndex(prev)
            }
            break
          }
          case 'Home': {
            e.preventDefault()
            if (visibleRows[0]) {
              setFocusedPath(visibleRows[0].file.path)
              rowVirtualizer.scrollToIndex(0)
            }
            break
          }
          case 'End': {
            e.preventDefault()
            if (visibleRows.length) {
              const last = visibleRows.length - 1
              setFocusedPath(visibleRows[last].file.path)
              rowVirtualizer.scrollToIndex(last)
            }
            break
          }
          case 'ArrowRight': {
            if (!current) break
            e.preventDefault()
            if (isDir) {
              const expanded = useFileTreeStore.getState().isExpanded(current.path)
              if (!expanded) {
                void toggleDirectory(current.path)
              } else {
                const child = visibleRows[curIndex + 1]
                if (child && child.depth === visibleRows[curIndex].depth + 1) {
                  setFocusedPath(child.file.path)
                  rowVirtualizer.scrollToIndex(curIndex + 1)
                }
              }
            }
            break
          }
          case 'ArrowLeft': {
            if (!current) break
            e.preventDefault()
            if (isDir && useFileTreeStore.getState().isExpanded(current.path)) {
              void toggleDirectory(current.path)
            } else {
              const sep = current.path.includes('\\') ? '\\' : '/'
              const parentPath = current.path.split(sep).slice(0, -1).join(sep)
              const parentIdx = visibleRows.findIndex((r) => r.file.path === parentPath)
              if (parentIdx >= 0) {
                setFocusedPath(parentPath)
                rowVirtualizer.scrollToIndex(parentIdx)
              }
            }
            break
          }
          case 'Enter': {
            if (!current) break
            e.preventDefault()
            if (isDir) {
              void toggleDirectory(current.path)
            } else {
              void Promise.resolve(onFileOpen?.(current.path, false))
            }
            break
          }
          case 'F2': {
            if (!current) break
            e.preventDefault()
            onRenamePath?.(current.path)
            break
          }
        }
      }}
      onDragOver={(e) => {
        e.preventDefault()
        e.dataTransfer.dropEffect = dragState.draggedItem ? 'move' : 'copy'
      }}
      onDrop={handleRootDrop}
      onClick={handleContainerClick}
      onDoubleClick={handleContainerDoubleClick}
      onContextMenu={handleContainerContextMenu}
      onMouseDown={handleContainerMouseDown}
      onMouseMove={handleContainerMouseMove}
      onMouseUp={handleContainerMouseUp}
      onMouseLeave={handleContainerMouseLeave}
    >
      <SidebarHeader onClick={(e) => e.stopPropagation()} onMouseDown={(e) => e.stopPropagation()}>
        <div className="flex items-stretch gap-1.5">
          <span className="relative flex min-w-0 flex-1 items-center">
            <Search
              aria-hidden="true"
              className="pointer-events-none absolute start-2.5 z-10 size-3.5 text-muted-foreground/72"
            />
            <Input
              nativeInput
              ref={searchInputRef}
              value={treeSearchQuery}
              onChange={(e) => setTreeSearchQuery(e.target.value)}
              size="sm"
              placeholder="Search"
              className="ps-5"
              name="file-tree-filter"
              aria-label="Filter files in tree"
              aria-controls="file-tree-results"
              autoCapitalize="none"
              autoComplete="off"
              autoCorrect="off"
              spellCheck="false"
              onKeyDown={(e) => {
                if (e.key === 'Escape') {
                  e.preventDefault()
                  e.stopPropagation()
                  closeTreeSearch()
                  return
                }

                if (e.key === 'Enter') {
                  e.preventDefault()
                  e.stopPropagation()
                  navigateTreeSearchMatch(e.shiftKey ? -1 : 1)
                }
              }}
            />
          </span>
          <Button
            ref={filterButtonRef}
            variant="outline"
            active={hasActiveFileTreeFilters}
            tooltip="Filter Files"
            tooltipSide="bottom"
            className={cn(
              'h-7.5 w-7.5 shrink-0 self-stretch rounded-lg p-0 sm:h-6.5 sm:w-6.5',
              // The theme's muted/accent tokens are ~4% alpha, so the default hover
              // just makes the button translucent over the glass sidebar. Use an
              // opaque mix of the popover base + foreground for a real muted fill.
              'hover:bg-[color-mix(in_oklch,var(--popover),var(--foreground)_10%)] dark:hover:bg-[color-mix(in_oklch,var(--popover),var(--foreground)_10%)]',
              'data-pressed:bg-[color-mix(in_oklch,var(--popover),var(--foreground)_16%)] dark:data-pressed:bg-[color-mix(in_oklch,var(--popover),var(--foreground)_16%)]',
              hasActiveFileTreeFilters && 'text-secondary',
            )}
            onClick={() => setIsFileTreeFilterMenuOpen(true)}
          >
            <Funnel className="size-3.5" />
          </Button>
        </div>
      </SidebarHeader>
      {!rootFolderPath ? (
        <div className="file-tree-empty-state flex flex-1 items-center justify-center">
          <SidebarEmptyActionState
            message="No folder open"
            actionLabel="Open Folder"
            onAction={handleOpenFolder}
          />
        </div>
      ) : displayedFiles.length === 0 ? (
        <div className="file-tree-empty-state flex flex-1 items-center justify-center">
          <SidebarEmptyActionState
            message={
              isTreeSearchSearching
                ? 'Searching files'
                : isTreeSearchActive
                  ? 'No matching files'
                  : isFileTreeLoading
                    ? 'Loading files…'
                    : 'Folder is empty'
            }
          />
        </div>
      ) : (
        <div id="file-tree-results" className="file-tree-scroll-body p-1">
          {(() => {
            const items = rowVirtualizer.getVirtualItems()
            const paddingTop = items.length ? items[0].start : 0
            const paddingBottom = items.length
              ? rowVirtualizer.getTotalSize() - items[items.length - 1].end
              : 0
            const densityConfig = FILE_TREE_DENSITY_CONFIG[fileTreeDensity]
            const stickyMarkerIndex =
              items.length && visibleRows.length
                ? Math.min(
                    visibleRows.length - 1,
                    Math.max(
                      0,
                      Math.floor((rowVirtualizer.scrollOffset ?? 0) / densityConfig.rowHeight),
                    ),
                  )
                : -1
            const stickyAncestors =
              stickyMarkerIndex >= 0 ? getStickyAncestorRows(visibleRows, stickyMarkerIndex) : []
            const stickyAncestorsStyle = {
              '--file-tree-container-inset': `${FILE_TREE_CONTAINER_INSET}px`,
              '--file-tree-header-height': `${FILE_TREE_HEADER_HEIGHT}px`,
              '--file-tree-sticky-row-height': `${densityConfig.rowHeight}px`,
              '--file-tree-sticky-stack-height': `${
                stickyAncestors.length * densityConfig.rowHeight
              }px`,
            } as React.CSSProperties
            return (
              <>
                {stickyAncestors.length > 0 ? (
                  <div className="file-tree-sticky-ancestors" style={stickyAncestorsStyle}>
                    <div className="file-tree-sticky-ancestor-stack">
                      {stickyAncestors.map((stickyAncestor) => {
                        const stickyAncestorLabel =
                          stickyAncestor.displayName ?? stickyAncestor.file.name
                        const stickyAncestorGitStatus = getGitStatusDecoration(stickyAncestor.file)
                        const stickyAncestorPaddingLeft =
                          FILE_TREE_BASE_INDENT +
                          FILE_TREE_CONTAINER_INSET +
                          stickyAncestor.depth * settings.fileTreeIndentSize

                        return (
                          <button
                            key={stickyAncestor.file.path}
                            type="button"
                            data-file-path={stickyAncestor.file.path}
                            data-is-dir={stickyAncestor.file.isDir}
                            data-path={stickyAncestor.file.path}
                            data-depth={stickyAncestor.depth}
                            title={stickyAncestor.file.path}
                            className={cn(
                              'file-tree-row ui-font ui-text-sm flex w-full min-w-max cursor-pointer select-none items-center whitespace-nowrap rounded-none border-none bg-transparent text-left text-foreground outline-none transition-colors duration-150 hover:bg-muted focus:outline-none',
                              densityConfig.rowClassName,
                            )}
                            style={{ paddingLeft: `${stickyAncestorPaddingLeft}px` }}
                          >
                            <FileExplorerIcon
                              fileName={stickyAncestor.file.name}
                              isDir={stickyAncestor.file.isDir ?? false}
                              isExpanded={stickyAncestor.isExpanded}
                              isSymlink={stickyAncestor.file.isSymlink}
                              className="relative z-1 shrink-0 text-muted-foreground"
                            />
                            <span
                              className={cn(
                                'relative z-1 select-none whitespace-nowrap',
                                stickyAncestorGitStatus?.colorClassName,
                              )}
                            >
                              {stickyAncestorLabel}
                            </span>
                          </button>
                        )
                      })}
                    </div>
                  </div>
                ) : null}
                <div style={{ height: paddingTop }} />
                {items.map((vi) => {
                  const row = visibleRows[vi.index]
                  const previousRow = visibleRows[vi.index - 1]
                  const nextRow = visibleRows[vi.index + 1]
                  const guideTargets: Array<FileTreeGuideTarget | null> =
                    guideTargetsByIndex[vi.index] ?? []
                  return (
                    <FileExplorerTreeItem
                      key={row.file.path}
                      file={row.file}
                      depth={row.depth}
                      displayName={row.displayName}
                      guideTargets={guideTargets}
                      previousDepth={previousRow?.depth ?? 0}
                      nextDepth={nextRow?.depth ?? 0}
                      indentSize={settings.fileTreeIndentSize}
                      density={fileTreeDensity}
                      isExpanded={row.isExpanded}
                      isActive={highlightedPath === row.file.path}
                      dragOverPath={dragState.dragOverPath}
                      isDragging={dragState.isDragging}
                      editingValue={editingValue}
                      onEditingValueChange={setEditingValue}
                      onKeyDown={handleKeyDown}
                      onBlur={handleBlur}
                      getGitStatusDecoration={getGitStatusDecoration}
                      rowId={getFileTreeRowId(row.file.path)}
                      searchQuery={isTreeSearchActive ? treeSearchQuery : undefined}
                      isSearchMatch={treeSearchResult.matchedPaths.has(row.file.path)}
                      fileFeedback={fileFeedback}
                    />
                  )
                })}
                <div style={{ height: paddingBottom }} />
              </>
            )
          })()}
        </div>
      )}

      {contextMenuElement}
      <Dropdown
        isOpen={isFileTreeFilterMenuOpen}
        anchorRef={filterButtonRef}
        anchorSide="bottom"
        anchorAlign="end"
        items={fileTreeFilterMenuItems}
        onClose={() => setIsFileTreeFilterMenuOpen(false)}
        closeOnSelect={false}
        className="w-fit min-w-fit"
      />
      <FileExplorerDialogs
        alertDialog={alertDialog}
        onCloseAlertDialog={() => setAlertDialog(null)}
        openAllFilesDialog={openAllFilesDialog}
        isOpeningAllFiles={isOpeningAllFiles}
        onCloseOpenAllFilesDialog={() => setOpenAllFilesDialog(null)}
        onConfirmOpenAllFiles={() => void handleOpenAllFilesConfirm()}
        deleteCandidate={deleteCandidate}
        isDeletingPath={isDeletingPath}
        onCloseDeleteDialog={() => setDeleteCandidate(null)}
        onConfirmDelete={() => void handleDeleteConfirm()}
        getPathBaseName={getPathBaseName}
      />
    </div>
  )
}

export const FileExplorerTree = memo(FileExplorerTreeComponent)
export default FileExplorerTree
