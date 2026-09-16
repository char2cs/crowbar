import { useLayoutEffect, useState, type RefObject } from 'react'

/**
 * Live height of `ref`'s element, measured synchronously on mount (so a
 * caller relying on it for first-paint layout — `IDEShell`'s own floating
 * file-explorer card, sized as a proportion of the sidebar rail — never sees
 * a one-frame-late zero) and kept current via `ResizeObserver`.
 */
export function useMeasuredHeight(ref: RefObject<HTMLElement | null>): number {
  const [height, setHeight] = useState(0)
  useLayoutEffect(() => {
    const el = ref.current
    if (!el) return
    const measure = () => setHeight(el.getBoundingClientRect().height)
    measure()
    if (typeof ResizeObserver === 'undefined') return
    const ro = new ResizeObserver(measure)
    ro.observe(el)
    return () => ro.disconnect()
  }, [ref])
  return height
}
