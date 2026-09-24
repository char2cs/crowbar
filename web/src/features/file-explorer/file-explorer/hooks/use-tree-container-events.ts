import type { Virtualizer } from '@tanstack/react-virtual'
import type React from 'react'
import { useCallback, useMemo, useRef, type RefObject } from 'react'
import { fileOpenBenchmark } from '@/features/editor/utils/file-open-benchmark'
import type { VisibleFileTreeRow } from '@/features/file-explorer/lib/visible-file-tree-rows'
import { useFileClipboardStore } from '@/features/file-explorer/stores/file-explorer-clipboard-store'
import { useFileTreeStore } from '@/features/file-explorer/stores/file-explorer-tree-store'
import type { FileEntry } from '@/features/file-system/types/app'
import { toast } from '@/features/window/stores/toast-store'

const DRAG_THRESHOLD_PX = 5

const parentPathOf = (path: string) => {
  const sep = path.includes('\\') ? '\\' : '/'
  return path.split(sep).slice(0, -1).join(sep)
}

interface TreeContainerEventsOptions {
  workspaceId: string | null
  rootFolderPath?: string
  containerRef: RefObject<HTMLDivElement | null>
  visibleRows: VisibleFileTreeRow[]
  rowVirtualizer: Pick<Virtualizer<HTMLDivElement, Element>, 'scrollToIndex'>
  keyboardPath: string | undefined
  setFocusedPath: (path: string | undefined) => void
  updateActivePath?: (path: string) => void
  onFileSelect: (path: string, isDir: boolean) => void | Promise<void>
  onFileOpen?: (path: string, isDir: boolean) => void | Promise<void>
  onRenamePath?: (path: string, newName?: string) => void
  onRefreshDirectory?: (path: string) => void
  openSearch: () => void
  handleContextMenu: (e: React.MouseEvent, path: string, isDir: boolean) => void
  closeContextMenu: () => void
  isDragging: boolean
  startDrag: (e: React.MouseEvent, file: FileEntry) => void
}

/** Paste really moves/copies, so it can fail (occupied destination is a 409); say so. */
function pasteInto(targetDir: string, workspaceId: string | null, onRefresh?: (p: string) => void) {
  useFileClipboardStore
    .getState()
    .actions.paste(targetDir, workspaceId)
    .then((results) => {
      onRefresh?.(targetDir)
      const failed = results.filter((result) => !result.success)
      if (failed.length > 0) {
        toast.error('Paste failed', failed.map((result) => result.destination_path).join(', '))
      }
    })
    .catch((error: unknown) => {
      toast.error('Paste failed', error instanceof Error ? error.message : String(error))
    })
}

/**
 * Delegated pointer and keyboard handlers for the tree container: rows carry
 * `data-file-path`/`data-is-dir`, so one set of handlers serves every
 * virtualized row.
 */
export function useTreeContainerEvents(o: TreeContainerEventsOptions) {
  const {
    workspaceId,
    rootFolderPath,
    containerRef,
    visibleRows,
    rowVirtualizer,
    keyboardPath,
    setFocusedPath,
    updateActivePath,
    onFileSelect,
    onFileOpen,
    onRenamePath,
    onRefreshDirectory,
    openSearch,
    handleContextMenu,
    closeContextMenu,
    isDragging,
    startDrag,
  } = o

  // Drag-threshold bookkeeping only — never rendered, so a ref avoids a
  // redraw on every mousedown/mousemove/mouseup during drag detection.
  const mouseDownInfoRef = useRef<{ x: number; y: number; file: FileEntry } | null>(null)

  const pathToFile = useMemo(() => {
    const m = new Map<string, FileEntry>()
    for (const r of visibleRows) m.set(r.file.path, r.file)
    return m
  }, [visibleRows])

  const getTargetItem = useCallback(
    (target: EventTarget | null) => {
      const el = (target as HTMLElement | null)?.closest<HTMLElement>('[data-file-path]')
      if (!el) return null
      const path = el.dataset.filePath ?? ''
      const isDir = el.dataset.isDir === 'true'
      const file = pathToFile.get(path)
      return file ? { path, isDir, file } : null
    },
    [pathToFile],
  )

  const toggleDirectory = useCallback(
    (path: string) => void Promise.resolve(onFileSelect(path, true)),
    [onFileSelect],
  )

  const focusRow = useCallback(
    (index: number) => {
      const path = visibleRows[index]?.file.path
      if (!path) return
      setFocusedPath(path)
      rowVirtualizer.scrollToIndex(index)
    },
    [rowVirtualizer, setFocusedPath, visibleRows],
  )

  const onClick = useCallback(
    (e: React.MouseEvent) => {
      const t = getTargetItem(e.target)
      e.preventDefault()
      e.stopPropagation()
      if (!t) {
        setFocusedPath(undefined)
        updateActivePath?.('')
        return
      }
      setFocusedPath(t.path)
      if (t.isDir) {
        toggleDirectory(t.path)
        updateActivePath?.(t.path)
      } else {
        fileOpenBenchmark.ensureStarted(t.path, 'explorer-click')
        fileOpenBenchmark.mark(t.path, 'explorer-click')
        void Promise.resolve(onFileSelect(t.path, false))
      }
    },
    [getTargetItem, onFileSelect, setFocusedPath, toggleDirectory, updateActivePath],
  )

  // Double-click begins an inline rename (onRenamePath with no new name marks
  // the node editable); the context-menu "Rename" uses the same path.
  const onDoubleClick = useCallback(
    (e: React.MouseEvent) => {
      const t = getTargetItem(e.target)
      if (!t) return
      e.preventDefault()
      e.stopPropagation()
      setFocusedPath(t.path)
      onRenamePath?.(t.path)
    },
    [getTargetItem, onRenamePath, setFocusedPath],
  )

  const onContextMenu = useCallback(
    (e: React.MouseEvent) => {
      const t = getTargetItem(e.target)
      if (t) handleContextMenu(e, t.path, t.isDir)
      else if (rootFolderPath) handleContextMenu(e, rootFolderPath, true)
    },
    [getTargetItem, handleContextMenu, rootFolderPath],
  )

  const onMouseDown = useCallback(
    (e: React.MouseEvent) => {
      if (e.button !== 0) return
      const t = getTargetItem(e.target)
      if (t) mouseDownInfoRef.current = { x: e.clientX, y: e.clientY, file: t.file }
    },
    [getTargetItem],
  )

  const onMouseMove = useCallback(
    (e: React.MouseEvent) => {
      const info = mouseDownInfoRef.current
      if (!info || isDragging) return
      if (Math.hypot(e.clientX - info.x, e.clientY - info.y) > DRAG_THRESHOLD_PX) {
        startDrag(e, info.file)
        mouseDownInfoRef.current = null
      }
    },
    [isDragging, startDrag],
  )

  const clearMouseDown = useCallback(() => {
    mouseDownInfoRef.current = null
  }, [])

  const onKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      const mod = e.metaKey || e.ctrlKey
      if (
        (mod && e.key.toLowerCase() === 'f') ||
        (!mod && !e.altKey && !e.shiftKey && e.key === '/')
      ) {
        e.preventDefault()
        e.stopPropagation()
        openSearch()
        return
      }

      // Let inputs handle their own keys
      const target = e.target as HTMLElement
      if (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.isContentEditable) {
        return
      }
      const index = visibleRows.findIndex((r) => r.file.path === keyboardPath)
      const curIndex = index === -1 ? 0 : index
      const current = visibleRows[curIndex]?.file
      const isDir = !!current?.isDir
      const isExpanded = () =>
        !!current && useFileTreeStore.getState().isExpanded(workspaceId ?? '', current.path)

      if (mod && current) {
        const clipboard = useFileClipboardStore.getState().actions
        if (e.key === 'c' || e.key === 'x') {
          e.preventDefault()
          clipboard[e.key === 'c' ? 'copy' : 'cut']([{ path: current.path, is_dir: isDir }])
          return
        }
        if (e.key === 'v') {
          e.preventDefault()
          const targetDir = isDir ? current.path : parentPathOf(current.path)
          if (targetDir) pasteInto(targetDir, workspaceId, onRefreshDirectory)
          return
        }
      }

      switch (e.key) {
        case 'Escape':
          e.preventDefault()
          e.stopPropagation()
          closeContextMenu()
          containerRef.current?.focus()
          break
        case 'ArrowDown':
          e.preventDefault()
          focusRow(Math.min(visibleRows.length - 1, curIndex + 1))
          break
        case 'ArrowUp':
          e.preventDefault()
          focusRow(Math.max(0, curIndex - 1))
          break
        case 'Home':
          e.preventDefault()
          focusRow(0)
          break
        case 'End':
          e.preventDefault()
          focusRow(visibleRows.length - 1)
          break
        case 'ArrowRight': {
          if (!current) break
          e.preventDefault()
          if (!isDir) break
          if (!isExpanded()) {
            toggleDirectory(current.path)
          } else if (visibleRows[curIndex + 1]?.depth === visibleRows[curIndex].depth + 1) {
            focusRow(curIndex + 1)
          }
          break
        }
        case 'ArrowLeft': {
          if (!current) break
          e.preventDefault()
          if (isDir && isExpanded()) {
            toggleDirectory(current.path)
          } else {
            const parentPath = parentPathOf(current.path)
            const parentIdx = visibleRows.findIndex((r) => r.file.path === parentPath)
            if (parentIdx >= 0) focusRow(parentIdx)
          }
          break
        }
        case 'Enter':
          if (!current) break
          e.preventDefault()
          if (isDir) toggleDirectory(current.path)
          else void Promise.resolve(onFileOpen?.(current.path, false))
          break
        case 'F2':
          if (!current) break
          e.preventDefault()
          onRenamePath?.(current.path)
          break
      }
    },
    [
      closeContextMenu,
      containerRef,
      focusRow,
      keyboardPath,
      onFileOpen,
      onRefreshDirectory,
      onRenamePath,
      openSearch,
      toggleDirectory,
      visibleRows,
      workspaceId,
    ],
  )

  return {
    onClick,
    onDoubleClick,
    onContextMenu,
    onMouseDown,
    onMouseMove,
    onMouseUp: clearMouseDown,
    onMouseLeave: clearMouseDown,
    onKeyDown,
  }
}
