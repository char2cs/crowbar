import { useEffect } from 'react'
import type { RefObject } from 'react'
import { isTauri } from '@/lib/crowbar-bridge'
import { shellChannel } from '@/sdk/shell-channel'

interface TauriDropPosition {
  x: number
  y: number
}

function pointInRect(position: TauriDropPosition, rect: DOMRect): boolean {
  return (
    position.x >= rect.left &&
    position.x <= rect.right &&
    position.y >= rect.top &&
    position.y <= rect.bottom
  )
}

interface RegisteredConsumer {
  containerRef: RefObject<HTMLElement | null>
  onDrop: (paths: string[]) => void
}

const consumers = new Set<RegisteredConsumer>()

interface Hit {
  consumer: RegisteredConsumer
  el: HTMLElement
}

function areaOf(el: HTMLElement): number {
  const rect = el.getBoundingClientRect()
  return Math.max(0, rect.right - rect.left) * Math.max(0, rect.bottom - rect.top)
}

function pickBySpecificity(hits: Hit[]): RegisteredConsumer | undefined {
  if (hits.length === 0) return undefined
  return hits.reduce((champion, candidate) => {
    if (champion.el.contains(candidate.el)) return candidate
    if (candidate.el.contains(champion.el)) return champion
    return areaOf(candidate.el) < areaOf(champion.el) ? candidate : champion
  }).consumer
}

function hitsWhere(inside: (el: HTMLElement) => boolean): Hit[] {
  const hits: Hit[] = []
  for (const consumer of consumers) {
    const el = consumer.containerRef.current
    if (el && inside(el)) hits.push({ consumer, el })
  }
  return hits
}

function resolveConsumer(position: TauriDropPosition): RegisteredConsumer | undefined {
  if (typeof document.elementFromPoint === 'function') {
    const elAtPoint = document.elementFromPoint(position.x, position.y)
    return elAtPoint ? pickBySpecificity(hitsWhere((el) => el.contains(elAtPoint))) : undefined
  }
  return pickBySpecificity(hitsWhere((el) => pointInRect(position, el.getBoundingClientRect())))
}

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
      stop()
      return
    }
    sharedUnlisten = stop
  })()
}

export function deliverShellFileDrop(position: TauriDropPosition, paths: string[]): boolean {
  const consumer = resolveConsumer(position)
  consumer?.onDrop(paths)
  return consumer !== undefined
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

export function useTauriFileDrop(
  containerRef: RefObject<HTMLElement | null>,
  onDrop: (paths: string[]) => void,
): void {
  useEffect(() => {
    const tauri = isTauri()
    if (!tauri && !shellChannel()) return
    const self: RegisteredConsumer = { containerRef, onDrop }
    consumers.add(self)
    if (tauri) ensureSharedSubscription()

    return () => {
      consumers.delete(self)
      releaseSharedSubscriptionIfIdle()
    }
  }, [containerRef, onDrop])
}
