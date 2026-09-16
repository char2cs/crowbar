import { useStore } from 'zustand'
import { useShallow } from 'zustand/react/shallow'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import type { WindowPaneState } from '@/features/panes/stores/window-pane-store.types'
import type { BufferActions } from '@/features/panes/stores/slices/buffer-slice'
import type { PaneContent } from '@/features/panes/types/pane-content'

/** Task 26: buffers are window-level now — see use-pane-store.ts's own note. */
function useBufferStore<T>(selector: (state: WindowPaneState) => T): T {
  return useStore(windowPaneStore, selector)
}

export const useBufferActions = (): BufferActions => useBufferStore((s) => s.bufferActions)

export const useBufferById = (id: string): PaneContent | undefined =>
  useBufferStore((s) => s.buffers.find((b) => b.id === id))

export const useBuffersByIds = (ids: string[]): PaneContent[] =>
  useBufferStore(
    useShallow((s) => {
      const map = new Map(s.buffers.map((b) => [b.id, b]))
      return ids.flatMap((id) => {
        const buffer = map.get(id)
        return buffer ? [buffer] : []
      })
    }),
  )
