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
  // The offset still OWED to `key`: set when a restore was clamped by content
  // too short to hold it, cleared once reached or once the user takes over.
  // While it is non-null, every offset the element reports is one of our own
  // clamped attempts rather than where the user is reading, so nothing is
  // recorded. It deliberately outlives the effect that created it — see the
  // StrictMode note on the restore effect below.
  const pendingTargetRef = useRef<number | null>(null)

  // Record every scroll, and capture a final offset when this key is torn down.
  // A LAYOUT effect (not a passive one) so that on an in-place key change the
  // outgoing key's cleanup runs BEFORE the restore effect below moves the shared
  // element to the incoming key's offset — otherwise the outgoing buffer would
  // be handed the incoming buffer's position.
  useLayoutEffect(() => {
    const container = containerRef.current
    if (!container || !key) return

    const handleScroll = () => {
      if (pendingTargetRef.current !== null) return
      rememberScroll(key, container.scrollTop)
    }
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
      // Same reasoning while a restore is still owed: the element is sitting at
      // a clamped fraction of the offset we're trying to reach.
      if (pendingTargetRef.current !== null) return
      // A detached node also reports 0, for the same reason.
      if (container.isConnected) rememberScroll(key, container.scrollTop)
    }
  }, [containerRef, key])

  // Restore before paint so the surface never flashes at the top. The offset is
  // APPLIED once per key — re-applying after the user has scrolled would yank
  // them back — but the watcher that chases a clamped restore is re-established
  // on every run, which is what makes this survive StrictMode (setup → cleanup
  // → setup: the first cleanup tears the watcher down, and the second setup is
  // past the once-per-key guard, so a watcher owned by that guard would be dead
  // on arrival and the offset would stay truncated).
  useLayoutEffect(() => {
    const container = containerRef.current
    if (!container || !key || !isContentReady) return

    if (restoredKeyRef.current !== key) {
      restoredKeyRef.current = key
      pendingTargetRef.current = null

      const target = getPreservedScroll(key)
      container.scrollTop = target
      // A CLAMPED assignment means the document is still shorter than the offset
      // we're restoring into: images, embedded HTML and diagrams size themselves
      // after the first commit, and a browser silently truncates a scrollTop
      // beyond the current scroll range. Landing on the target (which includes
      // the target-is-0 case) leaves nothing owed.
      if (container.scrollTop < target) pendingTargetRef.current = target
    }

    const target = pendingTargetRef.current
    if (target === null || typeof ResizeObserver === 'undefined') return

    // Growth is observed, not polled: a ResizeObserver fires exactly when a box
    // changes, so this costs nothing once the document settles and there is no
    // frame budget to guess wrong. It self-terminates — reaching the target, or
    // the content simply not growing any further, ends it.
    let applied = container.scrollTop

    // Detach the watcher but keep the target owed, so a re-run re-establishes it.
    const detach = () => {
      observer.disconnect()
      container.removeEventListener('scroll', onScroll)
    }
    // Nothing left to chase: reached, or the user took the wheel.
    const settle = () => {
      pendingTargetRef.current = null
      detach()
    }

    // Our own re-applies land exactly on `applied`; any other value is the user
    // scrolling while the content is still loading, and chasing the old offset
    // would yank them back mid-read.
    const onScroll = () => {
      if (container.scrollTop !== applied) settle()
    }

    const observer = new ResizeObserver(() => {
      container.scrollTop = target
      applied = container.scrollTop
      if (applied >= target) settle()
    })

    container.addEventListener('scroll', onScroll, { passive: true })
    // The container's own box is sized by its parent; what grows is the content
    // inside it, so observe the children too.
    observer.observe(container)
    for (const child of Array.from(container.children)) observer.observe(child)

    return detach
  }, [containerRef, key, isContentReady])
}
