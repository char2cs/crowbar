// Stub
import { create } from "zustand"
import { createSelectors } from "@/utils/zustand-selectors"

interface ZoomState {
  zoom: number
  editorZoomLevel: number
  terminalZoomLevel: number
  setZoom: (z: number) => void
  setEditorZoomLevel: (z: number) => void
  setTerminalZoomLevel: (z: number) => void
  resetZoom: () => void
}

const useZoomStoreBase = create<ZoomState>((set) => ({
  zoom: 1,
  editorZoomLevel: 0,
  terminalZoomLevel: 0,
  setZoom: (z) => set({ zoom: z }),
  setEditorZoomLevel: (z) => set({ editorZoomLevel: z }),
  setTerminalZoomLevel: (z) => set({ terminalZoomLevel: z }),
  resetZoom: () => set({ zoom: 1, editorZoomLevel: 0, terminalZoomLevel: 0 }),
}))

export const useZoomStore = createSelectors(useZoomStoreBase)
