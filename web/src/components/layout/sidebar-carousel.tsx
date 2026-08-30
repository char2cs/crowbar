import { useCallback, useEffect, useRef, useState, Suspense } from 'react'
import type { PointerEvent as ReactPointerEvent } from 'react'
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
import {
  DEFAULT_CARD_HEIGHT_FRACTION,
  clampCardHeightFraction,
  loadCardHeightFraction,
  saveCardHeightFraction,
} from './sidebar-card-height'

// 'workspaces' and 'chats' are both dropped: spec §6.1's card holds two
// glyphs and nothing else, Files and Git. Part B's SidebarTree and Part D's
// RecentsBand own the workspaces surface directly, mounted via
// `SpaceScroller` in `sidebar-tree-surface.tsx` (see task-30-report.md) —
// above this carousel, not as one of its panels.
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
  /**
   * Height (px) of the sidebar rail this card floats over (spec §6) —
   * undefined before `ide-shell.tsx`'s own ResizeObserver has measured it
   * once. The card opens at one third of this, then remembers a user drag as
   * a proportion of it (see sidebar-card-height.ts) so it survives a window
   * resize instead of holding a stale pixel value.
   */
  sidebarHeight?: number
  /**
   * Reports the card's own live height (px) on every change — mount, prop
   * resize, and every frame of a drag. `ide-shell.tsx` uses this to give the
   * tree region below an equal bottom inset (spec §6: "the tree keeps a
   * bottom inset the height of the card") without the tree independently
   * assuming any number of its own — this card is the one source of truth
   * for its own height.
   */
  onHeightChange?: (heightPx: number) => void
}

export function SidebarCarousel({
  activeWorkspaceRepoPath,
  sidebarHeight,
  onHeightChange,
}: SidebarCarouselProps) {
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
  const cardRef = useRef<HTMLDivElement>(null)
  // The user's own committed open height, as a proportion of `sidebarHeight`
  // — spec §6: "height is kept as a proportion of the rail, so it survives a
  // window resize." Defaults to one third (the spec's own open default) on a
  // first run with nothing persisted yet.
  const [heightFraction, setHeightFraction] = useState(loadCardHeightFraction)
  const cardHeightPx =
    sidebarHeight != null && sidebarHeight > 0
      ? Math.round(sidebarHeight * heightFraction)
      : undefined

  // Reported on every change (mount, sidebarHeight resize, drag) so an
  // ancestor can give the tree an equal bottom inset without assuming any
  // number of its own — see the prop doc above.
  useEffect(() => {
    if (cardHeightPx != null) onHeightChange?.(cardHeightPx)
  }, [cardHeightPx, onHeightChange])

  // Pointer-drag resize from the top 6px hot zone (spec §6). Mirrors
  // pane-sash.tsx's/sidebar-split-pane.tsx's own pattern — track window
  // pointermove/up from pointerdown, coalesce the live size to one DOM write
  // (and one `onHeightChange` report, since the tree's bottom inset tracks
  // this same value live) per animation frame, commit once on release —
  // rather than importing pane-sash.tsx itself: that component drags a
  // flex-basis between two sibling panes, this drags one floating element's
  // own height against a rail it does not share layout with.
  const activeDragCleanupRef = useRef<(() => void) | null>(null)

  const handleResizePointerDown = useCallback(
    (e: ReactPointerEvent<HTMLDivElement>) => {
      if (e.button !== 0) return
      const rail = sidebarHeight
      if (!rail || rail <= 0) return
      e.preventDefault()
      const startY = e.clientY
      const startHeight = cardHeightPx ?? Math.round(rail * DEFAULT_CARD_HEIGHT_FRACTION)
      let liveHeight = startHeight
      let moved = false
      let animationFrame = 0

      const applyLiveHeight = () => {
        animationFrame = 0
        if (cardRef.current) cardRef.current.style.height = `${liveHeight}px`
        onHeightChange?.(liveHeight)
      }

      const teardown = () => {
        window.removeEventListener('pointermove', onMove)
        window.removeEventListener('pointerup', onUp)
        window.removeEventListener('pointercancel', onUp)
        if (animationFrame !== 0) cancelAnimationFrame(animationFrame)
        activeDragCleanupRef.current = null
        if (moved) {
          document.documentElement.removeAttribute('data-pane-resizing')
          window.dispatchEvent(new CustomEvent('pane-resize-end'))
        }
      }

      const onMove = (ev: globalThis.PointerEvent) => {
        if (!moved) {
          moved = true
          document.documentElement.setAttribute('data-pane-resizing', '1')
        }
        // The card is anchored to the RAIL's bottom edge, so dragging the top
        // edge up (negative delta) grows it.
        const delta = ev.clientY - startY
        const raw = startHeight - delta
        const clampedFraction = clampCardHeightFraction(raw / rail)
        liveHeight = Math.round(rail * clampedFraction)
        if (animationFrame === 0) animationFrame = requestAnimationFrame(applyLiveHeight)
      }

      const onUp = () => {
        const wasMoved = moved
        if (animationFrame !== 0) {
          cancelAnimationFrame(animationFrame)
          applyLiveHeight()
        }
        teardown()
        if (!wasMoved) return
        const fraction = clampCardHeightFraction(liveHeight / rail)
        setHeightFraction(fraction)
        saveCardHeightFraction(fraction)
      }

      activeDragCleanupRef.current = teardown
      window.addEventListener('pointermove', onMove)
      window.addEventListener('pointerup', onUp)
      window.addEventListener('pointercancel', onUp)
    },
    [sidebarHeight, cardHeightPx, onHeightChange],
  )

  // Unmount-mid-drag safety (mirrors pane-sash.tsx): stray window listeners
  // and the global resizing attribute must not survive this component going
  // away — e.g. the sidebar auto-collapsing, or a route change, mid-drag.
  useEffect(() => {
    return () => activeDragCleanupRef.current?.()
  }, [])

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
    // Floats over the tree, never splits layout with it (spec §6): absolute
    // within ide-shell.tsx's own `relative` sidebar column, inset 8px
    // (`inset-x-2 bottom-2`) on three sides — top is the resize handle
    // below, not an inset. `bg-pane-background`/the pane-content shadow are
    // the SAME ground and elevation `pane-container.tsx` casts onto the
    // sidebar from a docked pane; `rounded-lg` is `--radius`, not a
    // hand-rolled value.
    <div
      ref={cardRef}
      data-testid="carousel-card"
      className="absolute inset-x-2 bottom-2 z-10 flex flex-col overflow-hidden rounded-lg border bg-pane-background shadow-[0_3px_8px_rgba(0,0,0,0.24)]"
      style={cardHeightPx != null ? { height: `${cardHeightPx}px` } : undefined}
    >
      {/* Top 6px hot zone (spec §6) — matches pane-sash.tsx's own
          `h-1.5`/`w-1.5` literally rather than a new value. Lands on the
          card's own top edge (already drawn by its rounded corners/border),
          not a separate visible sash. */}
      <div
        data-testid="carousel-resize-handle"
        onPointerDown={handleResizePointerDown}
        className="absolute inset-x-0 top-0 z-10 h-1.5 cursor-row-resize touch-none"
      />
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
    </div>
  )
}
