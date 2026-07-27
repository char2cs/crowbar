import { useLayoutEffect, useRef, type RefObject } from 'react'
import { EDITOR_CONSTANTS } from '@/features/editor/config/constants'

/**
 * Retained scroll offsets for surfaces that are UNMOUNTED whenever their tab
 * isn't the pane's active one.
 *
 * PaneContainer renders only the active buffer (terminals and agent chats are
 * the two keep-alive exceptions), so a preview pane's DOM — and with it its
 * scroll position — is destroyed on every tab switch and rebuilt from scratch
 * on the way back. Monaco panes don't need this: their retained per-pane widget
 * restores a per-model view state. Everything else needs the offset to live
 * OUTSIDE the component, which is what this module is.
 *
 * Keyed by buffer id (not path), so the offset follows the buffer when it moves
 * between panes and two previews of different files never share one. Bounded
 * like the editor view-state cache — oldest entry evicted first.
 */
const MAX_ENTRIES = EDITOR_CONSTANTS.MAX_POSITION_CACHE_SIZE

const scrollOffsets = new Map<string, number>()

function rememberScroll(key: string, scrollTop: number): void {
  // Re-insert so the map's iteration order stays least-recently-used first.
  scrollOffsets.delete(key)
  scrollOffsets.set(key, scrollTop)
  if (scrollOffsets.size > MAX_ENTRIES) {
    const oldest = scrollOffsets.keys().next().value
    if (oldest !== undefined) scrollOffsets.delete(oldest)
  }
}

/** Remembered offset for `key`, or 0 when nothing was recorded. */
export function getPreservedScroll(key: string): number {
  return scrollOffsets.get(key) ?? 0
}

/** Drops one key's offset, or every offset when called with no key. */
export function clearPreservedScroll(key?: string): void {
  if (key === undefined) scrollOffsets.clear()
  else scrollOffsets.delete(key)
}

/**
 * Keeps `containerRef`'s scroll offset across unmount/remount cycles.
 *
 * @param containerRef the scrolling element
 * @param key          stable identity of the content (a buffer id); `null`
 *                     disables preservation entirely
 * @param isContentReady whether the scrollable content is in the DOM. Restoring
 *                     before it is would be clamped to 0 by the browser (an
 *                     empty scroll box has no room to scroll) and the offset
 *                     would be silently lost.
 */
export function usePreservedScroll(
  containerRef: RefObject<HTMLElement | null>,
  key: string | null,
  isContentReady: boolean,
): void {
  const restoredKeyRef = useRef<string | null>(null)

  // Record every scroll, and capture a final offset when this key is torn down.
  // A LAYOUT effect (not a passive one) so that on an in-place key change the
  // outgoing key's cleanup runs BEFORE the restore effect below moves the shared
  // element to the incoming key's offset — otherwise the outgoing buffer would
  // be handed the incoming buffer's position.
  useLayoutEffect(() => {
    const container = containerRef.current
    if (!container || !key) return

    const handleScroll = () => rememberScroll(key, container.scrollTop)
    container.addEventListener('scroll', handleScroll, { passive: true })

    return () => {
      container.removeEventListener('scroll', handleScroll)
      // Only capture a final offset for a key this element is actually SHOWING.
      // A cleanup can fire against a freshly mounted element that hasn't been
      // restored yet — StrictMode double-invokes every effect (setup → cleanup →
      // setup), and the preview's content arrives a commit after mount — and
      // recording that element's 0 would wipe the very offset the restore is
      // about to read. The scroll listener above has already banked every real
      // scroll, so skipping here loses nothing.
      if (restoredKeyRef.current !== key) return
      // A detached node also reports 0, for the same reason.
      if (container.isConnected) rememberScroll(key, container.scrollTop)
    }
  }, [containerRef, key])

  // Restore before paint so the surface never flashes at the top. Once per key:
  // re-running it after the user has scrolled would yank them back.
  useLayoutEffect(() => {
    const container = containerRef.current
    if (!container || !key || !isContentReady) return
    if (restoredKeyRef.current === key) return
    restoredKeyRef.current = key
    container.scrollTop = getPreservedScroll(key)
  }, [containerRef, key, isContentReady])
}
