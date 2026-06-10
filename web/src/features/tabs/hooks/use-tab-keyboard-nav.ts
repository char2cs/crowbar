import { useCallback } from 'react'
import type { MutableRefObject } from 'react'
import type { PaneContent } from '@/features/panes/types/pane-content'

interface UseTabKeyboardNavOptions {
  sortedBuffers: PaneContent[]
  tabRefs: MutableRefObject<(HTMLDivElement | null)[]>
  onTabClick: (bufferId: string) => void
  onUpdateActivePath: (path: string) => void
  onAnnounce: (message: string) => void
  onCloseTab: (bufferId: string) => void
}

/**
 * Returns a `handleKeyDown` callback for tab-bar keyboard navigation.
 * Supports ArrowLeft/ArrowRight, Home/End, Delete/Backspace, Enter/Space.
 */
export function useTabKeyboardNav({
  sortedBuffers,
  tabRefs,
  onTabClick,
  onUpdateActivePath,
  onAnnounce,
  onCloseTab,
}: UseTabKeyboardNavOptions) {
  return useCallback(
    (e: React.KeyboardEvent, index: number) => {
      const buffer = sortedBuffers[index]
      if (!buffer) return

      const announce = (buf: PaneContent) =>
        `Switched to ${buf.name}${buf.type === 'editor' && buf.isDirty ? ', unsaved changes' : ''}`

      switch (e.key) {
        case 'ArrowLeft':
          e.preventDefault()
          if (index > 0) {
            const prev = sortedBuffers[index - 1]
            onTabClick(prev.id)
            onUpdateActivePath(prev.path)
            onAnnounce(announce(prev))
            tabRefs.current[index - 1]?.focus()
          }
          break

        case 'ArrowRight':
          e.preventDefault()
          if (index < sortedBuffers.length - 1) {
            const next = sortedBuffers[index + 1]
            onTabClick(next.id)
            onUpdateActivePath(next.path)
            onAnnounce(announce(next))
            tabRefs.current[index + 1]?.focus()
          }
          break

        case 'Home':
          e.preventDefault()
          if (sortedBuffers.length > 0) {
            const first = sortedBuffers[0]
            onTabClick(first.id)
            onUpdateActivePath(first.path)
            onAnnounce(announce(first))
            tabRefs.current[0]?.focus()
          }
          break

        case 'End':
          e.preventDefault()
          if (sortedBuffers.length > 0) {
            const lastIndex = sortedBuffers.length - 1
            const last = sortedBuffers[lastIndex]
            onTabClick(last.id)
            onUpdateActivePath(last.path)
            onAnnounce(announce(last))
            tabRefs.current[lastIndex]?.focus()
          }
          break

        case 'Delete':
        case 'Backspace':
          if (!buffer.isPinned) {
            e.preventDefault()
            onAnnounce(`Closed ${buffer.name}`)
            onCloseTab(buffer.id)
          } else {
            onAnnounce(`Cannot close pinned tab ${buffer.name}`)
          }
          break

        case 'Enter':
        case ' ':
          e.preventDefault()
          onTabClick(buffer.id)
          onUpdateActivePath(buffer.path)
          onAnnounce(
            `Activated ${buffer.name}${buffer.type === 'editor' && buffer.isDirty ? ', unsaved changes' : ''}`,
          )
          break
      }
    },
    [sortedBuffers, onTabClick, onUpdateActivePath, onAnnounce, onCloseTab, tabRefs],
  )
}
