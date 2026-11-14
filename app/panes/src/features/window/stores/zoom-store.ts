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

// Rounded after every step, or IEEE-754 error accumulates into `style.zoom` (1.2000000000000002).
const ZOOM_STEP = 0.1
// Divide by 10 rather than multiply by STEP: multiplying back by 0.1 can reintroduce the error.
const roundToStep = (z: number) => Math.round(z * 10) / 10

const useZoomStoreBase = create<ZoomState>((set) => ({
  zoom: 1,
  editorZoomLevel: 1,
  terminalZoomLevel: 1,
  actions: {
    zoomIn: () => set((s) => ({ zoom: Math.min(roundToStep(s.zoom + ZOOM_STEP), 3) })),
    zoomOut: () => set((s) => ({ zoom: Math.max(roundToStep(s.zoom - ZOOM_STEP), 0.3) })),
    resetZoom: () => set({ zoom: 1, editorZoomLevel: 1, terminalZoomLevel: 1 }),
    setZoom: (z) => set({ zoom: z }),
    setEditorZoomLevel: (z) => set({ editorZoomLevel: z }),
    setTerminalZoomLevel: (z) => set({ terminalZoomLevel: z }),
  },
}))

export const useZoomStore = createSelectors(useZoomStoreBase)
