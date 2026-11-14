import type { WindowPaneState } from '../../window-pane-store.types'

/** The immer `set` the slice hands every action group: mutate the draft. */
export type PaneSet = (recipe: (state: WindowPaneState) => void) => void
export type PaneGet = () => WindowPaneState
