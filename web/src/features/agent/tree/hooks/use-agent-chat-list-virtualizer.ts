import { useRef } from 'react'
import { useVirtualizer } from '@tanstack/react-virtual'

/**
 * The fixed vertical pitch of one row: ROW_BASE's `h-9` (36px) plus its `my-0.5`
 * (4px). Every row in this tree is that tall — chats and folders alike, and a
 * row indented four levels no less than a root one — which is what lets the list
 * stay windowed as a tree: `estimateSize` is exact, so nothing is ever measured.
 */
export const AGENT_CHAT_ROW_HEIGHT = 40

/**
 * Windows the agent-chats sidebar list: only the visible slice (+ overscan) of
 * rows is mounted, so the DOM cost is bounded by the viewport, not the chat
 * count. The returned `scrollRef` must go on the `overflow-auto` scroll
 * container — the virtualizer owns that element directly, the same way the file
 * tree and git lists do.
 *
 * Rows are a fixed AGENT_CHAT_ROW_HEIGHT pitch, so `estimateSize` is exact and
 * no per-row measurement is needed.
 */
export function useAgentChatListVirtualizer(count: number) {
  const scrollRef = useRef<HTMLDivElement>(null)

  const rowVirtualizer = useVirtualizer({
    count,
    estimateSize: () => AGENT_CHAT_ROW_HEIGHT,
    getScrollElement: () => scrollRef.current,
    overscan: 8,
    // Mirror useFileExplorerVisibleRows: while a sidebar/pane drag is in progress
    // (data-pane-resizing on <html>) suppress ResizeObserver callbacks — otherwise
    // the drag drives 120fps re-renders — and flush the last deferred measurement
    // on the pane-resize-end event.
    observeElementRect: (instance, cb) => {
      const element = instance.scrollElement
      if (!element) return
      let pending: { width: number; height: number } | null = null
      const ro = new ResizeObserver(([entry]) => {
        const { width, height } = entry.contentRect
        const rect = { width: Math.round(width), height: Math.round(height) }
        if (document.documentElement.hasAttribute('data-pane-resizing')) {
          pending = rect
          return
        }
        cb(rect)
        pending = null
      })
      ro.observe(element)
      const r = element.getBoundingClientRect()
      cb({ width: Math.round(r.width), height: Math.round(r.height) })
      const flush = () => {
        if (pending) {
          cb(pending)
          pending = null
        }
      }
      window.addEventListener('pane-resize-end', flush)
      return () => {
        ro.disconnect()
        window.removeEventListener('pane-resize-end', flush)
      }
    },
  })

  return { scrollRef, rowVirtualizer }
}
