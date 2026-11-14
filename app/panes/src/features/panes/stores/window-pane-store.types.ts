// web/src/features/panes/stores/window-pane-store.types.ts
import type { PaneSlice } from './slices/pane-slice'
import type { BufferSlice } from './slices/buffer-slice'

/** One layout and buffer list for the whole window, never destroyed by a
 *  workspace's lifecycle; per-workspace live resources stay on `WorkspaceState`. */
export type WindowPaneState = PaneSlice & BufferSlice
