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
import type { PaneSet } from './pane-actions/context'
import type { PaneContent } from '@/features/panes/types/pane-content'
import { disposeBuffers, releaseUnreferencedBuffers } from '@/features/panes/lib/buffer-release'
import { commitViewWrite } from '@/features/panes/lib/view-ops'

type PaneDropZone = 'center' | 'left' | 'right' | 'top' | 'bottom'

interface OpenChatOptions {
  projectId?: string
  workspaceId?: string | null
  runnerId?: string | null
}

export interface PaneActions {
  splitPane(
    paneId: string,
    direction: SplitDirection,
    bufferId?: string,
    placement?: SplitPlacement,
  ): string | null
  openChat(chatId: string, opts?: OpenChatOptions): void
  dropChatOnPane(
    chatId: string,
    paneId: string,
    zone: PaneDropZone,
    workspaceId?: string | null,
  ): void
  detachPane(paneId: string): void
  closePane(paneId: string): void
  closeView(viewId: string): void
  reorderView(viewId: string, targetId: string, mode: 'before' | 'after'): void
  setViewPinned(viewId: string, pinned: boolean): void
  forgetChat(chatId: string): void
  retargetPane(paneId: string, chatId: string, runnerId: string | null): void
  setPaneRunner(paneId: string, runnerId: string | null): void
  tagDraftThread(chatId: string, parentId: string): void
  placeRestoredMembers(placed: ReadonlyMap<string, string>, gone: ReadonlySet<string>): void
  adoptBackgroundChat(chatId: string, projectId: string, workspaceId?: string | null): void
  activateView(viewId: string): void
  setActivePane(paneId: string): void
  activateEditorTabInPane(paneId: string, tabId: string): void
  activateChatInPane(paneId: string): void
  addEditorTabToPane(paneId: string, tab: EditorTabBase): void
  removeEditorTabFromPane(paneId: string, tabId: string): void
  moveEditorTabToPane(tabId: string, fromPaneId: string, toPaneId: string): void
  replaceEditorTab(fromId: string, toId: string): void
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
> = (rawSet, get) => {
  const set: PaneSet = (recipe) => {
    let released: PaneContent[] = []
    rawSet((state) => {
      commitViewWrite(state, recipe)
      released = releaseUnreferencedBuffers(state)
    })
    disposeBuffers(released)
  }
  return {
    ...initialViewState(),
    activeProjectId: null,
    paneActions: {
      ...createViewActions(set, get),
      ...createChatDropActions(set),
      ...createRunnerActions(set),
      ...createLayoutActions(set, get),
      ...createEditorTabActions(set, get),
    },
  }
}
