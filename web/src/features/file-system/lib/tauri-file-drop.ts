import { useEffect } from 'react'
import type { RefObject } from 'react'
import { isTauri } from '@/lib/crowbar-bridge'

export interface TauriDropPosition {
  x: number
  y: number
}

/** Pure — is `position` (Tauri's CSS-pixel, window-relative drop coordinate)
 *  inside `rect`. Inclusive of the boundary. */
export function pointInRect(position: TauriDropPosition, rect: DOMRect): boolean {
  return (
    position.x >= rect.left &&
    position.x <= rect.right &&
    position.y >= rect.top &&
    position.y <= rect.bottom
  )
}

export interface RegisteredConsumer {
  containerRef: RefObject<HTMLElement | null>
  onDrop: (paths: string[]) => void
}

// Every mounted `useTauriFileDrop` instance registers itself here for the
// lifetime of its effect (see the hook below), so a single drop can be
// arbitrated ACROSS instances — see `resolveConsumer` — instead of every
// instance reacting to it in isolation.
const consumers = new Set<RegisteredConsumer>()

function areaOf(el: HTMLElement): number {
  const rect = el.getBoundingClientRect()
  return Math.max(0, rect.right - rect.left) * Math.max(0, rect.bottom - rect.top)
}

// Among consumers whose container all geometrically contain the drop, picks
// the single most specific one: a descendant beats its ancestor (composer
// pill beats the pane wrapper it sits inside; a terminal buffer beats the
// pane hosting it), and — for two containers with no DOM relationship at all,
// e.g. a `Dialog`'s portal rendered outside the pane tree entirely — the
// smaller one (so a modal's own dropzone beats the whole pane behind it).
//
// Exported standalone (same reasoning as `pointInRect` above) so its two
// defensive branches — a candidate whose ref has already gone null, and two
// registrations that resolve to the identical DOM node — can be exercised
// directly: `resolveConsumer`'s own two call sites already pre-filter both
// away, so neither is reachable by mounting real `useTauriFileDrop` hooks.
export function pickBySpecificity(
  candidates: RegisteredConsumer[],
): RegisteredConsumer | undefined {
  if (candidates.length === 0) return undefined
  return candidates.reduce((champion, candidate) => {
    const championEl = champion.containerRef.current
    const candidateEl = candidate.containerRef.current
    if (!championEl || !candidateEl) return champion
    if (championEl === candidateEl) return champion
    if (championEl.contains(candidateEl)) return candidate
    if (candidateEl.contains(championEl)) return champion
    return areaOf(candidateEl) < areaOf(championEl) ? candidate : champion
  })
}

// Resolves which single registered consumer — if any — should handle a real
// drop at `position`. Prefers `document.elementFromPoint`: it is the actual
// browser hit-test, so it already accounts for stacking order (a `Dialog`'s
// portal drawn on top of the pane) and visibility (a backgrounded, hidden
// terminal buffer sharing its visible sibling's `absolute inset-0` rect is
// never hit-tested) — things a pure rect comparison can't see. jsdom does not
// implement `elementFromPoint` at all, so tests exercise the geometric
// fallback below, which is also what runs if a point ever falls outside any
// rendered element.
function resolveConsumer(position: TauriDropPosition): RegisteredConsumer | undefined {
  if (typeof document.elementFromPoint === 'function') {
    const elAtPoint = document.elementFromPoint(position.x, position.y)
    const hits = elAtPoint
      ? Array.from(consumers).filter((consumer) => {
          const el = consumer.containerRef.current
          return !!el && el.contains(elAtPoint)
        })
      : []
    return pickBySpecificity(hits)
  }

  const rectHits = Array.from(consumers).filter((consumer) => {
    const el = consumer.containerRef.current
    return !!el && pointInRect(position, el.getBoundingClientRect())
  })
  return pickBySpecificity(rectHits)
}

// A single, module-level Tauri subscription shared by every mounted consumer
// — not one per hook instance. Tauri's `onDragDropEvent` is a window-wide
// broadcast regardless, so N independent subscriptions bought nothing but N
// redundant IPC listeners; collapsing to one also gives `resolveConsumer` a
// single dispatch point to arbitrate from. Ref-counted via `consumers.size`:
// the subscription opens on the first mount and closes once the last
// consumer unmounts. `generation` guards the unmount-before-subscribe-
// resolves race — see `releaseSharedSubscriptionIfIdle`.
let sharedUnlisten: (() => void) | undefined
let sharedSubscribePromise: Promise<void> | undefined
let generation = 0

function ensureSharedSubscription(): void {
  if (sharedSubscribePromise) return
  const myGeneration = ++generation
  sharedSubscribePromise = (async () => {
    const { getCurrentWebview } = await import('@tauri-apps/api/webview')
    const stop = await getCurrentWebview().onDragDropEvent((event) => {
      if (event.payload.type !== 'drop') return
      resolveConsumer(event.payload.position)?.onDrop(event.payload.paths)
    })
    if (myGeneration !== generation) {
      // Every consumer unmounted (bumping `generation`) before this
      // resolved — there is nothing left to hand the subscription to.
      stop()
      return
    }
    sharedUnlisten = stop
  })()
}

function releaseSharedSubscriptionIfIdle(): void {
  if (consumers.size > 0) return
  generation += 1
  sharedSubscribePromise = undefined
  if (sharedUnlisten) {
    sharedUnlisten()
    sharedUnlisten = undefined
  }
}

/**
 * Real OS file paths dropped on `containerRef`'s element — Tauri desktop only.
 *
 * Tauri's webview intercepts a native OS file drag before it becomes a DOM
 * `drop` event at all: `dragDropEnabled` defaults to true and nothing in
 * `desktop/src-tauri/tauri.conf.json` overrides it, so `DataTransfer` never
 * carries files for a real drag and no DOM `drop` handler ever fires for
 * one. `extractDroppedFilePaths` (file-system-dropped-paths.ts) covers only
 * the other, unavoidably path-less case — a plain browser tab, where a
 * `DataTransfer` exists but no browser exposes a real host path on a `File`
 * either way. This hook is what actually answers "a real file got dropped
 * here, and here is its path" on the desktop build.
 *
 * Every mounted consumer would otherwise receive the SAME window-wide event
 * — Tauri does not scope `onDragDropEvent` to an element, and there is no DOM
 * event here to `stopPropagation()` on. Instead every instance registers
 * itself in a shared registry (see `resolveConsumer`) that a single shared
 * subscription (see `ensureSharedSubscription`) consults on each real drop to
 * decide which ONE registered consumer it belongs to — only that instance's
 * `onDrop` is called.
 */
export function useTauriFileDrop(
  containerRef: RefObject<HTMLElement | null>,
  onDrop: (paths: string[]) => void,
): void {
  useEffect(() => {
    if (!isTauri()) return
    const self: RegisteredConsumer = { containerRef, onDrop }
    consumers.add(self)
    ensureSharedSubscription()

    return () => {
      consumers.delete(self)
      releaseSharedSubscriptionIfIdle()
    }
  }, [containerRef, onDrop])
}
