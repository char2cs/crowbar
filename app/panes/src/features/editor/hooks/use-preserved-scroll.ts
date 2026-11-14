import { useLayoutEffect, useRef, type RefObject } from 'react'
import { EDITOR_CONSTANTS } from '@/features/editor/config/constants'

// Scroll offsets kept outside surfaces that unmount behind another tab (Monaco needs none: its
// retained widget restores per-model view state). Keyed by buffer id, so an offset follows its
// buffer across panes; bounded like the view-state cache, oldest evicted first.
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
function getPreservedScroll(key: string): number {
  return scrollOffsets.get(key) ?? 0
}

/** Drops one key's offset, or every offset when called with no key. */
export function clearPreservedScroll(key?: string): void {
  if (key === undefined) scrollOffsets.clear()
  else scrollOffsets.delete(key)
}

/** Keeps `containerRef`'s scroll offset across remounts, keyed by `key` (a buffer id; `null`
 *  disables it). Restore waits for `isContentReady`: an empty box clamps the offset to 0. */
export function usePreservedScroll(
  containerRef: RefObject<HTMLElement | null>,
  key: string | null,
  isContentReady: boolean,
): void {
  const restoredKeyRef = useRef<string | null>(null)
  // The offset still owed to `key` after a clamped restore, cleared once reached or the user takes
  // over. While set, reported offsets are our own clamped attempts, so none is recorded. It
  // outlives its effect on purpose (see the StrictMode note on the restore effect).
  const pendingTargetRef = useRef<number | null>(null)

  // Record every scroll, and a final offset at teardown. A layout effect, so on an in-place key
  // change the outgoing key's cleanup runs before the restore below moves the element.
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
      // Only bank a final offset for a key this element shows: a cleanup can hit a fresh,
      // unrestored element (StrictMode, late content), whose 0 would wipe the offset about to be
      // restored. The scroll listener already banked every real scroll.
      if (restoredKeyRef.current !== key) return
      // Same reasoning while a restore is still owed: the element is sitting at
      // a clamped fraction of the offset we're trying to reach.
      if (pendingTargetRef.current !== null) return
      // A detached node also reports 0, for the same reason.
      if (container.isConnected) rememberScroll(key, container.scrollTop)
    }
  }, [containerRef, key])

  // Restore before paint so nothing flashes at the top. The offset is applied once per key (later
  // would yank the user back), but the clamp-chasing watcher is rebuilt every run: under StrictMode
  // the second setup is past the once-per-key guard, so a guarded watcher would be dead on arrival.
  useLayoutEffect(() => {
    const container = containerRef.current
    if (!container || !key || !isContentReady) return

    if (restoredKeyRef.current !== key) {
      restoredKeyRef.current = key
      pendingTargetRef.current = null

      const target = getPreservedScroll(key)
      container.scrollTop = target
      // A clamped assignment means the document is still shorter than the offset (images, HTML
      // and diagrams size after the first commit). Landing on the target (or 0) leaves none owed.
      if (container.scrollTop < target) pendingTargetRef.current = target
    }

    const target = pendingTargetRef.current
    if (target === null || typeof ResizeObserver === 'undefined') return

    // Growth is observed with a ResizeObserver, not polled, so this costs nothing once settled. It
    // ends itself on reaching the target or when the content stops growing.
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
