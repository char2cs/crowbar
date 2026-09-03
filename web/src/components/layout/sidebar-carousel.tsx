import { useCallback, useEffect, useRef, Suspense } from 'react'
import { NavStack } from './nav-stack'
import { WorkspaceTree } from './workspace-tree'
import { FileExplorerTree } from '@/features/file-explorer/components/file-explorer-tree'
import { GitPanel } from '@/features/git/components/git-panel'
import { ErrorBoundary } from '@/components/error-boundary'
import { SidebarSkeleton } from './sidebar-skeleton'
import { useFileTreeStore } from '@/features/file-explorer/stores/file-explorer-tree-store'
import { useFileSystemStore } from '@/features/file-system/controllers/store'
import { pickAndUploadFiles } from '@/features/files/lib/file-upload'
import { useSidebarStore, type SidebarTab } from '@/lib/store/sidebar'
import { AgentChatsPanel } from '@/features/agent/tree/agent-chats-panel'

const TABS: SidebarTab[] = ['workspaces', 'chats', 'files', 'git']

interface SidebarCarouselProps {
  activeWorkspaceRepoPath: string
}

export function SidebarCarousel({ activeWorkspaceRepoPath }: SidebarCarouselProps) {
  const activeTab = useSidebarStore((s) => s.activeTab)
  const setActiveTab = useSidebarStore((s) => s.setActiveTab)
  const files = useFileSystemStore((s) => s.files)
  const handleFileOpen = useFileSystemStore.use.handleFileOpen?.()
  const handleFileSelect = useFileSystemStore.use.handleFileSelect?.()
  // File-tree mutation handlers (create/rename/delete/refresh) live on the
  // file-system store; thread them into the explorer so its context menu and
  // inline-edit actions actually run (the daemon backs them via /files).
  const setFiles = useFileSystemStore((s) => s.setFiles)
  const handleCreateNewFileInDirectory = useFileSystemStore.use.handleCreateNewFileInDirectory?.()
  const handleCreateNewFolderInDirectory =
    useFileSystemStore.use.handleCreateNewFolderInDirectory?.()
  const handleRenamePath = useFileSystemStore.use.handleRenamePath?.()
  const handleDeletePath = useFileSystemStore.use.handleDeletePath?.()
  const handleDuplicatePath = useFileSystemStore.use.handleDuplicatePath?.()
  const handleRevealInFolder = useFileSystemStore.use.handleRevealInFolder?.()
  const refreshDirectory = useFileSystemStore.use.refreshDirectory?.()
  const handleUploadFile = useCallback(
    (directoryPath: string) => void pickAndUploadFiles(directoryPath),
    [],
  )
  const containerRef = useRef<HTMLDivElement>(null)
  // Armed only by an actual scroll gesture over the carousel. Everything else
  // that moves scrollLeft is reflow, not intent: the re-align below, the
  // activeTab effect's smooth scroll, and — the one that bit — the browser
  // clamping the offset to 0 while the sidebar collapses to zero width and then
  // restoring it as the sidebar expands. Reading those offsets back through
  // Math.round() picked whatever panel happened to be nearest, so hiding and
  // showing the sidebar while on Files silently landed you on Chats.
  const isUserGesture = useRef(false)
  const armUserGesture = () => {
    isUserGesture.current = true
  }

  // Re-align scroll when the container is resized (sidebar separator drag,
  // sidebar collapse/expand, window resize). Each
  // carousel panel is min-w-full, so scrollLeft must stay at
  // tabIndex * containerWidth.
  useEffect(() => {
    const el = containerRef.current
    if (!el || typeof ResizeObserver === 'undefined') return
    const ro = new ResizeObserver(() => {
      isUserGesture.current = false
      const index = TABS.indexOf(useSidebarStore.getState().activeTab)
      if (index === -1) return
      // A collapsed sidebar has zero width: no offset identifies a panel, and
      // the browser has already clamped scrollLeft to 0. Leave it — the resize
      // that reopens the sidebar re-aligns it.
      if (el.clientWidth === 0) return
      el.scrollLeft = index * el.clientWidth
    })
    ro.observe(el)
    return () => ro.disconnect()
  }, [])

  // Scroll to the correct panel when activeTab changes (e.g. tab bar click)
  useEffect(() => {
    const el = containerRef.current
    if (!el) return
    const index = TABS.indexOf(activeTab)
    if (index === -1) return
    isUserGesture.current = false
    el.scrollTo({ left: index * el.clientWidth, behavior: 'smooth' })
  }, [activeTab])

  // Sync activeTab when the user swipes
  function handleScroll() {
    if (!isUserGesture.current) return
    const el = containerRef.current
    if (!el || el.clientWidth === 0) return
    const index = Math.round(el.scrollLeft / el.clientWidth)
    const tab = TABS[index]
    if (tab && tab !== useSidebarStore.getState().activeTab) {
      setActiveTab(tab)
    }
  }

  return (
    <NavStack>
      <div
        ref={containerRef}
        onScroll={handleScroll}
        onWheel={armUserGesture}
        onTouchStart={armUserGesture}
        data-sidebar-carousel=""
        className="flex flex-1 overflow-x-scroll overflow-y-hidden [scroll-snap-type:x_mandatory] [scrollbar-width:none] [&::-webkit-scrollbar]:hidden"
      >
        {/* Workspaces panel */}
        <div className="min-w-full [scroll-snap-align:start] flex flex-col overflow-hidden h-full">
          <WorkspaceTree />
        </div>

        {/* Chats panel */}
        <div className="min-w-full [scroll-snap-align:start] flex flex-col overflow-hidden h-full">
          <AgentChatsPanel />
        </div>

        {/* Files panel */}
        <div className="min-w-full [scroll-snap-align:start] flex flex-col overflow-hidden">
          <ErrorBoundary>
            <Suspense fallback={<SidebarSkeleton />}>
              <FileExplorerTree
                files={files}
                rootFolderPath={activeWorkspaceRepoPath}
                onFileSelect={(path, isDir) => {
                  if (isDir) {
                    useFileTreeStore.getState().toggleFolder(path)
                  } else {
                    handleFileSelect?.(path, false)
                  }
                }}
                onFileOpen={
                  handleFileOpen
                    ? (path: string, isDir: boolean) => {
                        if (!isDir) void handleFileOpen(path, false)
                      }
                    : undefined
                }
                onUpdateFiles={setFiles}
                onCreateNewFileInDirectory={handleCreateNewFileInDirectory ?? (() => {})}
                onCreateNewFolderInDirectory={handleCreateNewFolderInDirectory ?? undefined}
                onRenamePath={handleRenamePath ?? undefined}
                onDeletePath={handleDeletePath ?? undefined}
                onDuplicatePath={handleDuplicatePath ?? undefined}
                onRevealInFinder={handleRevealInFolder ?? undefined}
                onUploadFile={handleUploadFile}
                onRefreshDirectory={refreshDirectory ?? undefined}
              />
            </Suspense>
          </ErrorBoundary>
        </div>

        {/* Git panel */}
        <div className="min-w-full [scroll-snap-align:start] flex flex-col overflow-hidden">
          <GitPanel />
        </div>
      </div>
    </NavStack>
  )
}
