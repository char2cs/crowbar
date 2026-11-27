import type { WindowPaneState } from '../../window-pane-store.types'

export type PaneSet = (recipe: (state: WindowPaneState) => void) => void
export type PaneGet = () => WindowPaneState
