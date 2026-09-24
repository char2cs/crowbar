import type { StateCreator } from 'zustand'
import type { WindowPaneState } from '../window-pane-store.types'
import type { EditorTabBase } from '@/features/panes/types/pane-content'
import type { PaneGroup, SplitDirection, SplitPlacement } from '@/features/panes/types/pane'
import { initialViewState, type ViewState } from '@/features/panes/lib/view-state'
import { createViewActions } from './pane-actions/view-actions'
import { createChatDropActions } from './pane-actions/chat-drop-actions'
import { createRunnerActions } from './pane-actions/runner-actions'
import { createLayoutActions } from './pane-actions/layout-actions'
import { createEditorTabActions } from './pane-actions/editor-tab-actions'

export type PaneDropZone = 'center' | 'left' | 'right' | 'top' | 'bottom'

export interface OpenChatOptions {
  /** The chat's project; defaults to the active one. */
  projectId?: string
  runnerId?: string | null
}

/**
 * Only the actions marked "row" can add or remove a Recents row (a view
 * record); every structural write goes through `lib/view-ops.ts`.
 */
export interface PaneActions {
  /** A chatless pane carved out of `paneId`'s share, in `paneId`'s view. */
  splitPane(
    paneId: string,
    direction: SplitDirection,
    bufferId?: string,
    placement?: SplitPlacement,
  ): string | null
  /** Row: reveal the view holding `chatId`, else a new record (or the showing
   *  stage, promoted). Never replaces what a pane shows. */
  openChat(chatId: string, opts?: OpenChatOptions): void
  /** Row: fill a chatless pane or split the target — the chat joins the
   *  target's view; an already-open chat is moved. */
  dropChatOnPane(chatId: string, paneId: string, zone: PaneDropZone): void
  /** Row: a group member becomes a record of its own, right after the group. */
  detachPane(paneId: string): void
  /** Row: a view left without a chat is removed. */
  closePane(paneId: string): void
  /** Row. */
  closeView(viewId: string): void
  reorderView(viewId: string, targetId: string, mode: 'before' | 'after'): void
  /** Row: the daemon deleted `chatId`. */
  forgetChat(chatId: string): void
  /** Row: the runner in `paneId` walked into `chatId` (`moved` frame only). */
  retargetPane(paneId: string, chatId: string, runnerId: string | null): void
  setPaneRunner(paneId: string, runnerId: string | null): void
  /** Row: a chat turned working with no pane — a background record, not shown. */
  adoptBackgroundChat(chatId: string, projectId: string): void
  /** Put a record on screen (another project's is only remembered). */
  activateView(viewId: string): void
  setActivePane(paneId: string): void
  activateEditorTabInPane(paneId: string, tabId: string): void
  activateChatInPane(paneId: string): void
  addEditorTabToPane(paneId: string, tab: EditorTabBase): void
  removeEditorTabFromPane(paneId: string, tabId: string): void
  moveEditorTabToPane(tabId: string, fromPaneId: string, toPaneId: string): void
  setEditorTabPreview(paneId: string, tabId: string): void
  setEditorTabPinned(paneId: string, tabId: string, pinned: boolean): void
  setPaneLocked(paneId: string, locked: boolean): void
  reorderEditorTabs(paneId: string, tabId: string, targetIndex: number): void
  resizePaneSplit(splitId: string, sizes: [number, number]): void
  distributePaneSplit(splitId: string): void
  togglePaneFullscreen(paneId: string): void
  exitPaneFullscreen(): void
  getAllPaneGroups(): PaneGroup[]
  getPaneById(paneId: string): PaneGroup | null
  getPaneByEditorTabId(tabId: string): PaneGroup | null
  getActivePane(): PaneGroup | null
  clearEditorTabPreviewEverywhere(): void
  switchToNextEditorTab(paneId: string): void
  switchToPreviousEditorTab(paneId: string): void
  navigateToPane(direction: 'left' | 'right' | 'up' | 'down'): void
  /** The route's project changed. One writer: ide-shell's route effect. */
  setActiveProject(projectId: string): void
  closeViewsForProject(projectId: string): void
}

export interface PaneSlice extends ViewState {
  paneActions: PaneActions
}

export const createPaneSlice: StateCreator<
  WindowPaneState,
  [['zustand/immer', never]],
  [],
  PaneSlice
> = (set, get) => ({
  ...initialViewState(),
  activeProjectId: null,
  paneActions: {
    ...createViewActions(set, get),
    ...createChatDropActions(set),
    ...createRunnerActions(set),
    ...createLayoutActions(set, get),
    ...createEditorTabActions(set, get),
  },
})
