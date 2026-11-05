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

// Every mounted `useTauriFileDrop` registers here for its effect's lifetime, so one drop is
// arbitrated across instances (`resolveConsumer`) instead of each reacting in isolation.
const consumers = new Set<RegisteredConsumer>()

function areaOf(el: HTMLElement): number {
  const rect = el.getBoundingClientRect()
  return Math.max(0, rect.right - rect.left) * Math.max(0, rect.bottom - rect.top)
}

// The most specific consumer containing the drop: a descendant beats its ancestor, and of two
// unrelated containers (e.g. a Dialog portal outside the pane tree) the smaller wins. Exported to
// test its null-ref and same-node branches, which resolveConsumer's call sites pre-filter away.
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

// The one consumer (if any) for a drop at `position`. elementFromPoint is the real hit-test, so
// it respects stacking (a Dialog portal over the pane) and visibility (a hidden terminal buffer);
// the geometric fallback covers jsdom, which lacks it, and points outside any rendered element.
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

// One Tauri subscription shared by all consumers (onDragDropEvent is window-wide anyway), giving
// resolveConsumer one dispatch point. Ref-counted by `consumers.size`; `generation` guards an
// unmount before the subscribe resolves (see releaseSharedSubscriptionIfIdle).
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

/** Real OS file paths dropped on `containerRef`'s element. Tauri desktop only: its webview takes
 *  native file drags before any DOM `drop`. The event is window-wide, so the shared subscription
 *  calls only the one registered consumer `resolveConsumer` picks. */
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
