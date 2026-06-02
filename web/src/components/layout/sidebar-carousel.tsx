import { useEffect, useRef, Suspense } from 'react'
import { WorkspaceTree } from './workspace-tree'
import { ChatList } from '@/features/chats/components/chat-list'
import { FileExplorerTree } from '@/features/file-explorer/components/file-explorer-tree'
import { GitPanel } from '@/features/git/components/git-panel'
import { ErrorBoundary } from '@/components/ErrorBoundary'
import { SidebarSkeleton } from './SidebarSkeleton'
import { useFileTreeStore } from '@/features/file-explorer/stores/file-explorer-tree-store'
import { useFileSystemStore } from '@/features/file-system/controllers/store'
import { useSidebarStore, type SidebarTab } from '@/lib/store/sidebar'

const TABS: SidebarTab[] = ['workspaces', 'chats', 'files', 'git']

interface SidebarCarouselProps {
  activeWorkspaceRepoPath: string
}

export function SidebarCarousel({ activeWorkspaceRepoPath }: SidebarCarouselProps) {
  const activeTab = useSidebarStore(s => s.activeTab)
  const setActiveTab = useSidebarStore(s => s.setActiveTab)
  const files = useFileSystemStore(s => s.files)
  const handleFileOpen = useFileSystemStore.use.handleFileOpen?.()
  const handleFileSelect = useFileSystemStore.use.handleFileSelect?.()
  const containerRef = useRef<HTMLDivElement>(null)
  const isScrollingProgrammatically = useRef(false)

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
    <div
      ref={containerRef}
      onScroll={handleScroll}
      className="flex flex-1 overflow-x-scroll overflow-y-hidden [scroll-snap-type:x_mandatory] [scrollbar-width:none] [&::-webkit-scrollbar]:hidden"
    >
      {/* Workspaces panel */}
      <div className="min-w-full [scroll-snap-align:start] flex flex-col overflow-hidden">
        <WorkspaceTree />
      </div>

      {/* Chats panel */}
      <div className="min-w-full [scroll-snap-align:start] flex flex-col overflow-hidden">
        <ChatList />
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
              onFileOpen={handleFileOpen ? (path: string, isDir: boolean) => {
                if (!isDir) void handleFileOpen(path, false)
              } : undefined}
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
  )
}
