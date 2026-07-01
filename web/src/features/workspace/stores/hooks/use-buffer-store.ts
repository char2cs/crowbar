import { useShallow } from 'zustand/react/shallow'
import { useWorkspaceStoreContext } from '../workspace-context'
import type { BufferActions } from '../slices/buffer-slice'
import type { PaneContent } from '@/features/panes/types/pane-content'

export const useBuffers = (): PaneContent[] => useWorkspaceStoreContext((s) => s.buffers)

export const useBufferActions = (): BufferActions =>
  useWorkspaceStoreContext((s) => s.bufferActions)

export const useBufferById = (id: string): PaneContent | undefined =>
  useWorkspaceStoreContext((s) => s.buffers.find((b) => b.id === id))

export const useBuffersByIds = (ids: string[]): PaneContent[] =>
  useWorkspaceStoreContext(
    useShallow((s) => {
      const map = new Map(s.buffers.map((b) => [b.id, b]))
      return ids.map((id) => map.get(id)).filter(Boolean) as PaneContent[]
    }),
  )
