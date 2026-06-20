import { useEffect, useRef, Suspense } from 'react'
import { useRouterState } from '@tanstack/react-router'
import { NavStack } from './nav-stack'
import { WorkspaceTree } from './workspace-tree'
import { ChatTree } from './chat-tree'
import { FileExplorerTree } from '@/features/file-explorer/components/file-explorer-tree'
import { GitPanel } from '@/features/git/components/git-panel'
import { ErrorBoundary } from '@/components/error-boundary'
import { SidebarSkeleton } from './sidebar-skeleton'
import { useFileTreeStore } from '@/features/file-explorer/stores/file-explorer-tree-store'
import { useFileSystemStore } from '@/features/file-system/controllers/store'
import { useSidebarStore, type SidebarTab } from '@/lib/store/sidebar'
import { parseWorkspaceScopeFromPath } from '@/lib/workspace-scope'

const TABS: SidebarTab[] = ['workspaces', 'chats', 'files', 'git']

interface SidebarCarouselProps {
  activeWorkspaceRepoPath: string
}

export function SidebarCarousel({ activeWorkspaceRepoPath }: SidebarCarouselProps) {
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  const activeWorkspaceId = parseWorkspaceScopeFromPath(pathname)?.wsId ?? ''
  const activeTab = useSidebarStore((s) => s.activeTab)
  const setActiveTab = useSidebarStore((s) => s.setActiveTab)
  const files = useFileSystemStore((s) => s.files)
  const handleFileOpen = useFileSystemStore.use.handleFileOpen?.()
  const handleFileSelect = useFileSystemStore.use.handleFileSelect?.()
  const containerRef = useRef<HTMLDivElement>(null)
  const isScrollingProgrammatically = useRef(false)

  // Re-align scroll when the container is resized (sidebar panel drag via react-resizable-panels).
  // Each carousel panel is min-w-full, so scrollLeft must stay at tabIndex * containerWidth.
  useEffect(() => {
    const el = containerRef.current
    if (!el || typeof ResizeObserver === 'undefined') return
    const ro = new ResizeObserver(() => {
      const index = TABS.indexOf(useSidebarStore.getState().activeTab)
      if (index === -1) return
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
    isScrollingProgrammatically.current = true
    el.scrollTo({ left: index * el.clientWidth, behavior: 'smooth' })

    function onScrollEnd() {
      isScrollingProgrammatically.current = false
      el!.removeEventListener('scrollend', onScrollEnd)
    }
    el.addEventListener('scrollend', onScrollEnd)

    return () => {
      el.removeEventListener('scrollend', onScrollEnd)
    }
  }, [activeTab])

  // Sync activeTab when user swipes
  function handleScroll() {
    if (isScrollingProgrammatically.current) return
    const el = containerRef.current
    if (!el) return
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
      data-sidebar-carousel=""
      className="flex flex-1 overflow-x-scroll overflow-y-hidden [scroll-snap-type:x_mandatory] [scrollbar-width:none] [&::-webkit-scrollbar]:hidden"
    >
      {/* Workspaces panel */}
      <div className="min-w-full [scroll-snap-align:start] flex flex-col overflow-hidden h-full">
        <WorkspaceTree />
      </div>

      {/* Chats panel */}
      <div className="min-w-full [scroll-snap-align:start] flex flex-col overflow-hidden">
        <ChatTree wsId={activeWorkspaceId} />
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
              onCreateNewFileInDirectory={() => {}}
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
