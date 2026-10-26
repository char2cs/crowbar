import '@/features/file-explorer/file-explorer/styles/file-explorer-tree.css'
import { useEffect, useMemo, useRef, useState } from 'react'
import { useVirtualizer } from '@tanstack/react-virtual'
import { FolderSimpleIcon, MagnifyingGlassIcon, XCircleIcon } from '@phosphor-icons/react'
import { SidebarFileRow, TreeGuides } from '@/components/sidebar/sidebar-file-row'
import { TOUCH_ROW_METRICS } from '@/components/sidebar/lib/row-metrics'
import { ChatBranchHeaderView } from '@/features/tabs/components/chat-branch-header-view'
import { FileExplorerIcon } from '@/features/file-explorer/components/file-explorer-icon'
import { DirectoryChevron } from '@/features/file-explorer/file-explorer/components/file-explorer-tree-item'
import { GitStatusLetter } from '@/features/file-explorer/file-explorer/components/git-status-letter'
import {
  buildVisibleFileTreeRows,
  computeFileTreeSearchHits,
  filterFileTreeForFffHits,
} from '@/features/file-explorer/lib/visible-file-tree-rows'
import type { NativeFiles, NativeFilesEntry } from '@/lib/native-chrome'
import { postToShell, receiveShellSnapshots } from '@/lib/shell-page-bridge'
import { useSurfaceThemeColor } from '@/features/window/hooks/use-surface-theme-color'

const M = TOUCH_ROW_METRICS

function useVisibleRows(snapshot: NativeFiles, query: string) {
  return useMemo(() => {
    const searching = query.trim() !== ''
    const result = searching
      ? filterFileTreeForFffHits(snapshot.files, computeFileTreeSearchHits(snapshot.files, query))
      : null
    const rows = buildVisibleFileTreeRows(
      result?.files ?? snapshot.files,
      result?.expandedPaths ?? new Set(snapshot.expanded),
      { compactFolders: snapshot.compactFolders && !searching },
    )
    return rows as Array<(typeof rows)[number] & { file: NativeFilesEntry }>
  }, [snapshot, query])
}

/**
 * The IDE's file drawer: the workspace's tree from the main webview's snapshot, drawn with the
 * sidebar's file rows at the chat list's touch scale. Only the search box is its own; a fold or
 * a pick goes back as an intent.
 */
export function MobileFilesPage() {
  useSurfaceThemeColor()
  const [snapshot, setSnapshot] = useState<NativeFiles | null>(null)
  useEffect(() => receiveShellSnapshots<NativeFiles>('files', setSnapshot), [])
  const [query, setQuery] = useState('')

  return (
    <div className="flex h-dvh flex-col overflow-hidden bg-chrome-bg pt-[env(safe-area-inset-top)] text-foreground">
      {snapshot?.header ? (
        <>
          <header className="flex shrink-0 flex-col gap-2 px-4 pt-1 pb-2">
            <ChatBranchHeaderView model={snapshot.header} />
            <label className="flex h-9 items-center gap-2 rounded-[10px] bg-sidebar-element-idle px-2.5 text-muted-foreground">
              <MagnifyingGlassIcon aria-hidden="true" size={17} />
              <input
                type="search"
                aria-label="Search files"
                placeholder="Search"
                autoCapitalize="none"
                autoCorrect="off"
                spellCheck={false}
                enterKeyHint="search"
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                className="min-w-0 flex-1 bg-transparent text-[17px] text-foreground outline-none placeholder:text-muted-foreground [&::-webkit-search-cancel-button]:hidden"
              />
              {query && (
                <button
                  type="button"
                  aria-label="Clear search"
                  className="-mr-2.5 flex size-9 items-center justify-center"
                  onClick={() => setQuery('')}
                >
                  <XCircleIcon aria-hidden="true" size={17} weight="fill" />
                </button>
              )}
            </label>
          </header>
          <FilesList snapshot={snapshot} query={query} />
        </>
      ) : (
        snapshot && <NoWorkspace />
      )}
    </div>
  )
}

function NoWorkspace() {
  return (
    <div className="flex flex-1 flex-col items-center justify-center gap-2 px-8 pb-24 text-center">
      <FolderSimpleIcon aria-hidden="true" size={32} className="text-muted-foreground" />
      <p className="font-heading text-[19px] leading-6 tracking-tight">No workspace selected</p>
      <p className="text-[15px] text-muted-foreground">Open a chat to browse its files.</p>
    </div>
  )
}

function FilesList({ snapshot, query }: { snapshot: NativeFiles; query: string }) {
  const rows = useVisibleRows(snapshot, query)
  const scrollRef = useRef<HTMLDivElement>(null)
  const virtualizer = useVirtualizer({
    count: rows.length,
    estimateSize: () => M.height,
    getScrollElement: () => scrollRef.current,
    overscan: 10,
  })
  // `auto` scrolls only when the open file is off screen, so a fold above it never jumps.
  const activeIndex = rows.findIndex((row) => row.file.path === snapshot.activePath)
  useEffect(() => {
    if (activeIndex >= 0) virtualizer.scrollToIndex(activeIndex, { align: 'auto' })
  }, [activeIndex, virtualizer])

  const empty = rows.length === 0
  if (empty) {
    const text = snapshot.loading
      ? 'Loading files…'
      : snapshot.rootName === ''
        ? 'No workspace is open. Open a chat in a workspace to browse its files.'
        : query.trim()
          ? `No files match “${query.trim()}”`
          : 'No files'
    return <p className="px-4 pt-6 text-center text-[15px] text-muted-foreground">{text}</p>
  }

  return (
    <div
      ref={scrollRef}
      role="tree"
      aria-label="Files"
      className="file-tree-touch min-h-0 flex-1 overflow-y-auto pb-[env(safe-area-inset-bottom)] select-none [-webkit-touch-callout:none]"
    >
      <div className="relative w-full" style={{ height: virtualizer.getTotalSize() }}>
        {virtualizer.getVirtualItems().map((item) => {
          const row = rows[item.index]
          const { file, depth, isExpanded } = row
          return (
            <div
              key={file.path}
              className="absolute inset-x-0"
              style={{ top: item.start, height: item.size }}
            >
              <SidebarFileRow
                role="treeitem"
                aria-level={depth + 1}
                aria-expanded={file.isDir ? isExpanded : undefined}
                aria-selected={file.path === snapshot.activePath}
                data-path={file.path}
                depth={depth}
                indentSize={M.indent}
                baseIndent={M.padX}
                height={M.height}
                selected={file.path === snapshot.activePath}
                className={file.ignored ? 'opacity-50' : undefined}
                guides={
                  <TreeGuides
                    depth={depth}
                    baseIndent={M.padX}
                    indentSize={M.indent}
                    previousDepth={rows[item.index - 1]?.depth ?? 0}
                    nextDepth={rows[item.index + 1]?.depth ?? 0}
                  />
                }
                leading={<DirectoryChevron isDir={file.isDir} isExpanded={isExpanded} />}
                icon={
                  <FileExplorerIcon
                    fileName={file.name}
                    isDir={file.isDir}
                    isExpanded={isExpanded}
                    isSymlink={file.isSymlink}
                    className="shrink-0 text-muted-foreground"
                    size={M.glyph}
                  />
                }
                label={row.displayName ?? file.name}
                labelClassName={file.git?.colorClassName}
                badge={file.git && <GitStatusLetter decoration={file.git} />}
                onClick={() =>
                  postToShell('files', {
                    type: file.isDir ? 'toggleDir' : 'openFile',
                    path: file.path,
                  })
                }
              />
            </div>
          )
        })}
      </div>
    </div>
  )
}
