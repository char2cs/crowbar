import { useEffect, useRef } from 'react'
import type { RefObject } from 'react'
import { useSidebarStore } from '@/lib/store/sidebar'
import type { SidebarTab } from '@/lib/store/sidebar-ui'

// 'workspaces' and 'chats' are both dropped: spec §6.1's card holds two
// glyphs and nothing else, Files and Git. Part B's SidebarTree and Part D's
// RecentsBand own the workspaces surface directly, mounted via
// `SpaceScroller` in `sidebar-tree-surface.tsx` (see task-30-report.md) —
// above this carousel, not as one of its panels.
// A persisted activeTab of 'workspaces'/'chats' from before this change
// simply misses every entry here — TABS.indexOf returns -1, which every
// effect below already treats as a no-op.
const TABS: SidebarTab[] = ['files', 'git']

interface UseCarouselScrollSyncResult {
  armUserGesture: () => void
  handleScroll: () => void
}

/**
 * Keeps the carousel's horizontal scroll position and `activeTab` in sync in
 * both directions — split out of `SidebarCarousel` because it is a
 * self-contained scroll/observer loop over one container ref and the
 * sidebar store's own `activeTab`, untouched by the card's own resize/fold
 * state.
 */
export function useCarouselScrollSync(
  containerRef: RefObject<HTMLDivElement | null>,
  activeTab: SidebarTab,
  setActiveTab: (tab: SidebarTab) => void,
): UseCarouselScrollSyncResult {
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
  }, [containerRef])

  // Scroll to the correct panel when activeTab changes (e.g. tab bar click)
  useEffect(() => {
    const el = containerRef.current
    if (!el) return
    const index = TABS.indexOf(activeTab)
    if (index === -1) return
    isUserGesture.current = false
    el.scrollTo({ left: index * el.clientWidth, behavior: 'smooth' })
  }, [activeTab, containerRef])

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

  return { armUserGesture, handleScroll }
}
