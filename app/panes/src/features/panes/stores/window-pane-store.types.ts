import type { PaneSlice } from './slices/pane-slice'
import type { BufferSlice } from './slices/buffer-slice'

export type WindowPaneState = PaneSlice & BufferSlice
