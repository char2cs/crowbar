import type { Virtualizer } from '@tanstack/react-virtual'
import { useCallback, useEffect, useMemo, useRef, useState, type RefObject } from 'react'
import { useDebounce } from 'use-debounce'
import {
  computeFileTreeSearchHits,
  filterFileTreeForFffHits,
  type VisibleFileTreeRow,
} from '@/features/file-explorer/lib/visible-file-tree-rows'
import { useFileTreeStore } from '@/features/file-explorer/stores/file-explorer-tree-store'
import type { FileEntry } from '@/features/file-system/types/app'

const SEARCH_DEBOUNCE_MS = 80

/**
 * The tree's filter box: open state, the (debounced) query, the tree it
 * narrows the explorer to, and the ways to open it (`file-tree-open-search`
 * window event, Cmd/Ctrl+F while the tree is on screen).
 */
export function useTreeSearch({
  workspaceId,
  filteredFiles,
  containerRef,
}: {
  workspaceId: string | null
  filteredFiles: FileEntry[]
  containerRef: RefObject<HTMLDivElement | null>
}) {
  const [isOpen, setOpen] = useState(false)
  const [query, setQuery] = useState('')
  const [debouncedQuery] = useDebounce(query, SEARCH_DEBOUNCE_MS)
  const inputRef = useRef<HTMLInputElement>(null)
  const savedExpandedPathsRef = useRef<Set<string> | null>(null)

  const isActive = query.trim().length > 0
  const isSearching = isActive && query.trim() !== debouncedQuery.trim()
  const result = useMemo(
    () =>
      filterFileTreeForFffHits(
        filteredFiles,
        computeFileTreeSearchHits(filteredFiles, debouncedQuery),
      ),
    [filteredFiles, debouncedQuery],
  )
  const showResult = isActive && !isSearching
  const displayedFiles = useMemo(
    () => (showResult ? result.files : isActive ? [] : filteredFiles),
    [showResult, isActive, result.files, filteredFiles],
  )
  const displayedExpandedPaths = showResult ? result.expandedPaths : undefined

  useEffect(() => {
    if (!isOpen) return
    const rafId = requestAnimationFrame(() => {
      inputRef.current?.focus()
      inputRef.current?.select()
    })
    return () => cancelAnimationFrame(rafId)
  }, [isOpen])

  const close = useCallback(() => {
    setOpen(false)
    setQuery('')
    containerRef.current?.focus()
  }, [containerRef])

  useEffect(() => {
    const open = () => setOpen(true)
    window.addEventListener('file-tree-open-search', open)
    return () => window.removeEventListener('file-tree-open-search', open)
  }, [])

  // Cmd/Ctrl+F when the tree itself doesn't have DOM focus (a click that opened
  // a file moved focus into Monaco). The container's own onKeyDown handles the
  // chord first and calls preventDefault; a live visibility check keeps a
  // folded/scrolled-away tree (Files and Git both stay mounted in the sidebar
  // carousel) from stealing the shortcut.
  useEffect(() => {
    const handleGlobalKeyDown = (e: KeyboardEvent) => {
      if (e.defaultPrevented) return
      if (!(e.metaKey || e.ctrlKey) || e.key.toLowerCase() !== 'f') return
      const rect = containerRef.current?.getBoundingClientRect()
      if (!rect || rect.width === 0 || rect.height === 0) return
      e.preventDefault()
      setOpen(true)
    }
    document.addEventListener('keydown', handleGlobalKeyDown)
    return () => document.removeEventListener('keydown', handleGlobalKeyDown)
  }, [containerRef])

  // While searching, expand every directory so the lazy loader fetches their
  // children — otherwise files in unexpanded dirs are invisible to the search.
  // The pre-search expansion is restored when the query clears.
  useEffect(() => {
    const wsId = workspaceId ?? ''
    const store = useFileTreeStore.getState()
    if (debouncedQuery.trim()) {
      savedExpandedPathsRef.current ??= new Set(store.getExpandedPaths(wsId))
      store.expandAll(wsId, filteredFiles)
    } else if (savedExpandedPathsRef.current) {
      store.setExpandedPaths(wsId, savedExpandedPathsRef.current)
      savedExpandedPathsRef.current = null
    }
  }, [debouncedQuery, filteredFiles, workspaceId])

  return {
    isOpen,
    setOpen,
    query,
    setQuery,
    inputRef,
    close,
    isActive,
    isSearching,
    result,
    displayedFiles,
    displayedExpandedPaths,
  }
}

export type TreeSearch = ReturnType<typeof useTreeSearch>

/**
 * Keeps the keyboard cursor on a search match: jumps to the first match when
 * the cursor isn't on one, and steps between matches (Enter / Shift+Enter).
 */
export function useTreeSearchNavigation({
  search,
  visibleRows,
  rowVirtualizer,
  keyboardPath,
  setFocusedPath,
}: {
  search: TreeSearch
  visibleRows: VisibleFileTreeRow[]
  rowVirtualizer: Pick<Virtualizer<HTMLDivElement, Element>, 'scrollToIndex'>
  keyboardPath: string | undefined
  setFocusedPath: (path: string) => void
}) {
  const { isActive } = search
  const { matchedPaths, orderedMatchedPaths } = search.result

  const matchIndexes = useMemo(() => {
    if (!isActive || matchedPaths.size === 0) return []
    const rowIndexByPath = new Map(visibleRows.map((row, index) => [row.file.path, index]))
    const indexes: number[] = []
    for (const path of orderedMatchedPaths) {
      const index = rowIndexByPath.get(path)
      if (index !== undefined) indexes.push(index)
    }
    return indexes
  }, [isActive, matchedPaths, orderedMatchedPaths, visibleRows])

  const focusRow = useCallback(
    (index: number) => {
      const path = visibleRows[index]?.file.path
      if (!path) return
      setFocusedPath(path)
      rowVirtualizer.scrollToIndex(index, { align: 'auto' })
    },
    [rowVirtualizer, setFocusedPath, visibleRows],
  )

  useEffect(() => {
    if (matchIndexes.length === 0) return
    if (keyboardPath && matchedPaths.has(keyboardPath)) return
    focusRow(matchIndexes[0])
  }, [focusRow, keyboardPath, matchIndexes, matchedPaths])

  return useCallback(
    (direction: 1 | -1) => {
      if (matchIndexes.length === 0) return
      const currentIndex = keyboardPath
        ? visibleRows.findIndex((row) => row.file.path === keyboardPath)
        : -1
      const fallback = direction > 0 ? matchIndexes[0] : matchIndexes[matchIndexes.length - 1]
      const next =
        direction > 0
          ? matchIndexes.find((index) => index > currentIndex)
          : [...matchIndexes].reverse().find((index) => index < currentIndex)
      focusRow(next ?? fallback)
    },
    [focusRow, keyboardPath, matchIndexes, visibleRows],
  )
}
