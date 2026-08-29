import { useCallback, useEffect, useRef, Suspense } from 'react'
import { useMatch } from '@tanstack/react-router'
import { FolderOpen, GitBranch } from '@phosphor-icons/react'
import { cn } from '@/lib/utils'
import { NavStack } from './nav-stack'
import { Tabs, TabsList, TabsTab } from '@/components/ui/tabs'
import { FileExplorerTree } from '@/features/file-explorer/components/file-explorer-tree'
import { GitPanel } from '@/features/git/components/git-panel'
import { ErrorBoundary } from '@/components/error-boundary'
import { SidebarSkeleton } from './sidebar-skeleton'
import { useFileTreeStore } from '@/features/file-explorer/stores/file-explorer-tree-store'
import { useFileSystemStore } from '@/features/file-system/controllers/store'
import { pickAndUploadFiles } from '@/features/files/lib/file-upload'
import { useSidebarStore, type SidebarTab } from '@/lib/store/sidebar'

// 'workspaces' and 'chats' are both dropped: spec §6.1's card holds two
// glyphs and nothing else, Files and Git. Part B's SidebarTree (mounted
// today as SidebarTreePanel) and Part D's RecentsBand own the workspaces
// surface directly rather than as a carousel panel — not yet wired to a
// live mount point outside this carousel, a disclosed gap for whichever
// task gives them one (see task-15-report.md).
// A persisted activeTab of 'workspaces'/'chats' from before this change
// simply misses every entry here — TABS.indexOf returns -1, which every
// effect below already treats as a no-op.
const TABS: SidebarTab[] = ['files', 'git']

// The head's two glyphs (spec §6.1). Icon only, in TABS order.
const HEAD_TABS: {
  tab: SidebarTab
  label: string
  Icon: React.ComponentType<{ size: number; weight: 'fill' | 'regular' }>
}[] = [
  { tab: 'files', label: 'Files', Icon: FolderOpen },
  { tab: 'git', label: 'Git', Icon: GitBranch },
]

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

  // Git has no meaning without a repo, and the project-home route has no
  // active workspace — carried over verbatim from the old SidebarTabBar,
  // which is retired now that the head lives here (spec §6.1).
  const isHomeRoute = Boolean(useMatch({ from: '/_shell/ide/$projectId/home', shouldThrow: false }))
  useEffect(() => {
    if (isHomeRoute && activeTab === 'git') {
      setActiveTab('files')
    }
  }, [isHomeRoute, activeTab, setActiveTab])
  const visibleHeadTabs = isHomeRoute ? HEAD_TABS.filter((t) => t.tab !== 'git') : HEAD_TABS

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
      {/* The head (spec §6.1): underline variant, icon only, no labels, no
          divider. justify-start overrides the base tabs list's w-fit
          justify-center — the fold control that belongs on ml-auto here has
          no live home to relocate from today (see task-15-report.md), so
          this head reserves the layout but renders no fold affordance yet. */}
      <div className="flex shrink-0 items-center px-2 py-1">
        <Tabs value={activeTab} onValueChange={(v) => setActiveTab(v as SidebarTab)}>
          <TabsList variant="underline" data-testid="tabs-underline" className="justify-start">
            {visibleHeadTabs.map(({ tab, label, Icon }) => {
              const isActive = activeTab === tab
              return (
                <TabsTab
                  key={tab}
                  value={tab}
                  aria-label={label}
                  className={cn(
                    'h-7 flex-none justify-center px-2.5',
                    isActive ? 'text-foreground' : 'text-foreground/62',
                  )}
                >
                  <Icon size={16} weight={isActive ? 'fill' : 'regular'} />
                </TabsTab>
              )
            })}
          </TabsList>
        </Tabs>
      </div>
      <div
        ref={containerRef}
        onScroll={handleScroll}
        onWheel={armUserGesture}
        onTouchStart={armUserGesture}
        data-sidebar-carousel=""
        className="flex flex-1 overflow-x-scroll overflow-y-hidden [scroll-snap-type:x_mandatory] [scrollbar-width:none] [&::-webkit-scrollbar]:hidden"
      >
        {/* Files panel */}
        <div
          data-testid="carousel-panel"
          className="min-w-full [scroll-snap-align:start] flex flex-col overflow-hidden"
        >
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
        <div
          data-testid="carousel-panel"
          className="min-w-full [scroll-snap-align:start] flex flex-col overflow-hidden"
        >
          <GitPanel />
        </div>
      </div>
    </NavStack>
  )
}
