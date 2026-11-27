import { useLayoutEffect, useRef, type RefObject } from 'react'
import { EDITOR_CONSTANTS } from '@/features/editor/config/constants'

const MAX_ENTRIES = EDITOR_CONSTANTS.MAX_POSITION_CACHE_SIZE

const scrollOffsets = new Map<string, number>()

function rememberScroll(key: string, scrollTop: number): void {
  scrollOffsets.delete(key)
  scrollOffsets.set(key, scrollTop)
  if (scrollOffsets.size > MAX_ENTRIES) {
    const oldest = scrollOffsets.keys().next().value
    if (oldest !== undefined) scrollOffsets.delete(oldest)
  }
}

function getPreservedScroll(key: string): number {
  return scrollOffsets.get(key) ?? 0
}

export function usePreservedScroll(
  containerRef: RefObject<HTMLElement | null>,
  key: string | null,
  isContentReady: boolean,
): void {
  const restoredKeyRef = useRef<string | null>(null)
  const pendingTargetRef = useRef<number | null>(null)

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
      if (restoredKeyRef.current !== key) return
      if (pendingTargetRef.current !== null) return
      if (container.isConnected) rememberScroll(key, container.scrollTop)
    }
  }, [containerRef, key])

  useLayoutEffect(() => {
    const container = containerRef.current
    if (!container || !key || !isContentReady) return

    if (restoredKeyRef.current !== key) {
      restoredKeyRef.current = key
      pendingTargetRef.current = null

      const target = getPreservedScroll(key)
      container.scrollTop = target
      if (container.scrollTop < target) pendingTargetRef.current = target
    }

    const target = pendingTargetRef.current
    if (target === null || typeof ResizeObserver === 'undefined') return

    let applied = container.scrollTop

    const detach = () => {
      observer.disconnect()
      container.removeEventListener('scroll', onScroll)
    }
    const settle = () => {
      pendingTargetRef.current = null
      detach()
    }

    const onScroll = () => {
      if (container.scrollTop !== applied) settle()
    }

    const observer = new ResizeObserver(() => {
      container.scrollTop = target
      applied = container.scrollTop
      if (applied >= target) settle()
    })

    container.addEventListener('scroll', onScroll, { passive: true })
    observer.observe(container)
    for (const child of Array.from(container.children)) observer.observe(child)

    return detach
  }, [containerRef, key, isContentReady])
}
