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

// Steps are 0.1, so rounding to 1 decimal after every step is what keeps
// zoom on the exact grid a repeated zoomIn/zoomOut walks — without it,
// IEEE-754 accumulates real error onto a value written straight into
// `style={{ zoom: chatZoom }}`: 1.2000000000000002, 1.3000000000000003, and
// so on the more it's pressed.
const ZOOM_STEP = 0.1
// `Math.round(z * 10) / 10`, not `Math.round(z / STEP) * STEP` — the latter
// still lands on a different double than the `1.2` literal for some inputs
// (multiplying an integer BACK by 0.1 can reintroduce the same
// representation error this exists to remove). Scaling to a whole number,
// rounding, then dividing by the SAME power of ten is the standard,
// round-trip-safe way to snap a float to one decimal place in IEEE-754.
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
