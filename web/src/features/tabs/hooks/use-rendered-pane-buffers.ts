import { useStoreWithEqualityFn } from 'zustand/traditional'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import type { PaneContent } from '@/features/panes/types/pane-content'
import { sameRenderedBuffer } from '../components/tab-bar-item-utils'

function sameRenderedBufferList(a: PaneContent[], b: PaneContent[]): boolean {
  return a.length === b.length && a.every((buffer, i) => sameRenderedBuffer(buffer, b[i]))
}

/**
 * A pane's buffers, the same array until a field the strip paints changes: `s.buffers` is new on
 * every keystroke. The returned objects may be stale outside those fields; read live ones off
 * `windowPaneStore.getState()`. The equality lives in `isEqual`, not a ref the selector writes:
 * Zustand v5 may run a selector for renders React discards, and a self-caching one never settles.
 */
export function useRenderedPaneBuffers(paneBufferIds: string[]): PaneContent[] {
  return useStoreWithEqualityFn(
    windowPaneStore,
    (s) => {
      const map = new Map(s.buffers.map((b) => [b.id, b]))
      const next: PaneContent[] = []
      for (const id of paneBufferIds) {
        const b = map.get(id)
        if (b) next.push(b)
      }
      return next
    },
    sameRenderedBufferList,
  )
}
