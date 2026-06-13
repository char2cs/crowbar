// web/src/features/workspace/stores/workspace-store.ts
import { createStore, type StoreApi } from 'zustand'
import { immer } from 'zustand/middleware/immer'
import type { WorkspaceState } from './workspace-store.types'
import { createPaneSlice } from './slices/pane-slice'
import { createBufferSlice } from './slices/buffer-slice'
import { createLspSlice } from './slices/lsp-slice'
import { createTerminalSlice } from './slices/terminal-slice'
import { createFileWatcherSlice } from './slices/file-watcher-slice'
import { createRecentFilesSlice } from './slices/recent-files-slice'
import { createBranchReviewSlice } from './slices/branch-review-slice'
import { saveSessionToStore } from '@/features/editor/stores/buffer-session-persistence'
import { ModelRegistry } from '@/features/editor/lib/model-registry'
import { EditorManager, type BufferMeta } from '@/features/editor/lib/editor-manager'
import {
  createActiveEditorRegistry,
  type ActiveEditorRegistry,
} from '@/features/editor/lib/active-editor-context'
import {
  EDITOR_CREATE_OPTIONS,
  langForUri,
  realEditorApi,
  realModelApi,
} from '@/features/editor/lib/monaco-adapters'
import { uriToFsPath } from '@/features/editor/lib/editor-uri'

/**
 * Per-workspace, NON-REACTIVE editor handles attached to the store object.
 * They are NOT part of the reactive {@link WorkspaceState} (so they never
 * trigger subscriptions/persistence). Access via `store.editorManager` /
 * `store.modelRegistry`; disposed in `destroyWorkspaceStore`.
 */
export interface WorkspaceEditorHandles {
  readonly modelRegistry: ModelRegistry
  readonly editorManager: EditorManager
  /**
   * Per-workspace active-editor pub/sub registry. The per-pane editor
   * controller publishes the current ActiveEditorContext here on each buffer
   * swap; satellite UI (status bar, LSP overlays, etc.) subscribe by paneId
   * without re-rendering on a tab switch. Sits next to {@link editorManager}
   * as a NON-REACTIVE handle.
   */
  readonly activeEditorRegistry: ActiveEditorRegistry
}

export type WorkspaceStore = StoreApi<WorkspaceState> & WorkspaceEditorHandles

export type WorkspaceSnapshot = Partial<
  Pick<
    WorkspaceState,
    | 'panes'
    | 'rootLayout'
    | 'bottomLayout'
    | 'activePaneId'
    | 'fullscreenPaneId'
    | 'mostRecentActivePaneIds'
    | 'buffers'
    | 'recentFiles'
    | 'terminalLayout'
  >
>

export function createWorkspaceStore(wsId: string, snapshot?: WorkspaceSnapshot): WorkspaceStore {
  const store = createStore<WorkspaceState>()(
    immer(
      (set, get, api): WorkspaceState => ({
        workspaceId: wsId,
        ...createPaneSlice(set, get, api),
        ...createBufferSlice(set, get, api),
        ...createLspSlice(set, get, api),
        ...createTerminalSlice(set, get, api),
        ...createFileWatcherSlice(set, get, api),
        ...createRecentFilesSlice(set, get, api),
        ...createBranchReviewSlice(set, get, api),
        ...(snapshot ?? {}),
      }),
    ),
  )

  store.subscribe((state, prev) => {
    if (state.buffers === prev.buffers) return
    const activePane = state.panes[state.activePaneId] ?? null
    saveSessionToStore(state.buffers, activePane?.activeBufferId ?? null)
  })

  // One ModelRegistry + EditorManager per workspace (non-reactive handles).
  // `text(uri)` reads the buffer content for the file at that uri, or '' if it
  // isn't loaded; `lang(uri)` derives the Monaco language id from the path.
  const registry = new ModelRegistry(realModelApi())
  const meta: BufferMeta = {
    lang: (uri) => langForUri(uri),
    text: (uri) => {
      const fsPath = uriToFsPath(uri)
      const buf = store
        .getState()
        .buffers.find((b) => b.type === 'editor' && b.path === fsPath)
      return buf && 'content' in buf ? buf.content : ''
    },
  }
  const manager = new EditorManager(realEditorApi(EDITOR_CREATE_OPTIONS), registry, meta)
  const activeEditorRegistry = createActiveEditorRegistry()

  return Object.assign(store, {
    modelRegistry: registry,
    editorManager: manager,
    activeEditorRegistry,
  })
}
