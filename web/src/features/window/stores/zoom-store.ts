// Stub
import { create } from 'zustand'
import { createSelectors } from '@/utils/zustand-selectors'

interface ZoomActions {
  zoomIn: () => void
  zoomOut: () => void
  resetZoom: () => void
  setZoom: (z: number) => void
  setEditorZoomLevel: (z: number) => void
  setTerminalZoomLevel: (z: number) => void
}

interface ZoomState {
  zoom: number
  editorZoomLevel: number
  terminalZoomLevel: number
  actions: ZoomActions
}

const useZoomStoreBase = create<ZoomState>((set) => ({
  zoom: 1,
  editorZoomLevel: 1,
  terminalZoomLevel: 1,
  actions: {
    zoomIn: () => set((s) => ({ zoom: Math.min(s.zoom + 0.1, 3) })),
    zoomOut: () => set((s) => ({ zoom: Math.max(s.zoom - 0.1, 0.3) })),
    resetZoom: () => set({ zoom: 1, editorZoomLevel: 1, terminalZoomLevel: 1 }),
    setZoom: (z) => set({ zoom: z }),
    setEditorZoomLevel: (z) => set({ editorZoomLevel: z }),
    setTerminalZoomLevel: (z) => set({ terminalZoomLevel: z }),
  },
}))

export const useZoomStore = createSelectors(useZoomStoreBase)
