// web/src/features/workspace/stores/workspace-store.ts
import { createStore, type StoreApi } from 'zustand'
import { immer } from 'zustand/middleware/immer'
import type { WorkspaceState } from './workspace-store.types'
import { createLspSlice } from './slices/lsp-slice'
import { createTerminalSlice } from './slices/terminal-slice'
import { createFileWatcherSlice } from './slices/file-watcher-slice'
import { createRecentFilesSlice } from './slices/recent-files-slice'
import { createBranchReviewSlice } from './slices/branch-review-slice'
import { createAgentChatsSlice } from './slices/agent-chats-slice'
import { ModelRegistry } from '@/features/editor/lib/model-registry'
import { EditorManager, type BufferMeta } from '@/features/editor/lib/editor-manager'
import {
  createActiveEditorRegistry,
  type ActiveEditorRegistry,
} from '@/features/editor/lib/active-editor-context'
import { uriToFsPath } from '@/features/editor/lib/editor-uri'

/**
 * Per-workspace, NON-REACTIVE editor handles attached to the store object.
 * They are NOT part of the reactive {@link WorkspaceState} (so they never
 * trigger subscriptions/persistence). Access via `store.editorManager` /
 * `store.modelRegistry`; disposed in `destroyWorkspaceStore`.
 */
export interface WorkspaceEditorHandles {
  /**
   * Monaco-backed handles, LAZILY constructed. `undefined` until the first real
   * editor need arms them via {@link armEditor}. `monaco-editor` is a multi-MB
   * dependency; constructing these eagerly in `createWorkspaceStore` would pull
   * it onto the cold-launch static import chain (main.tsx → route tree →
   * pane-container → workspace-store) even when no editor is ever opened. The
   * generic, buffer-type-agnostic store logic (buffer-slice / pane-slice) reads
   * these only through `?.` (a no-op release when unarmed — there is nothing to
   * release before an editor has mounted), and every editor component runs
   * strictly AFTER {@link armEditor} has resolved (EditorPane gates its mount on
   * it), so treating them as possibly-undefined is safe for every call site.
   */
  readonly modelRegistry: ModelRegistry | undefined
  readonly editorManager: EditorManager | undefined
  /**
   * Arm the Monaco-backed {@link editorManager} / {@link modelRegistry} on the
   * first ACTUAL editor need. Dynamic-imports `monaco-adapters` (and thus
   * `monaco-editor`) and constructs the two handles. Idempotent and
   * concurrency-safe: repeat/overlapping calls share one construction and one
   * promise, so every EditorPane mount may `await` it freely. Resolves once the
   * handles are present; a no-op that resolves immediately once armed.
   */
  readonly armEditor: () => Promise<void>
  /**
   * Per-workspace active-editor pub/sub registry. The per-pane editor
   * controller publishes the current ActiveEditorContext here on each buffer
   * swap; satellite UI (status bar, LSP overlays, etc.) subscribe by paneId
   * without re-rendering on a tab switch. Sits next to {@link editorManager}
   * as a NON-REACTIVE handle. Monaco-free, so it stays EAGER — satellite UI can
   * read it without ever pulling in the editor.
   */
  readonly activeEditorRegistry: ActiveEditorRegistry
}

export type WorkspaceStore = StoreApi<WorkspaceState> & WorkspaceEditorHandles

export type WorkspaceSnapshot = Partial<Pick<WorkspaceState, 'recentFiles' | 'terminalLayout'>>

export function createWorkspaceStore(wsId: string, snapshot?: WorkspaceSnapshot): WorkspaceStore {
  const store = createStore<WorkspaceState>()(
    immer((set, get, api): WorkspaceState => ({
      workspaceId: wsId,
      ...createLspSlice(set, get, api),
      ...createTerminalSlice(set, get, api),
      ...createFileWatcherSlice(set, get, api),
      ...createRecentFilesSlice(set, get, api),
      ...createBranchReviewSlice(set, get, api),
      ...createAgentChatsSlice(set, get, api),
      ...(snapshot ?? {}),
    })),
  )

  // Note: buffer-session-persistence's saveSessionToStore now fires once, off
  // the window-level pane store (window-pane-store.ts) — panes/buffers moved
  // there in Task 26, so there is one flat buffer list for the whole window,
  // not one per workspace.

  // One ModelRegistry + EditorManager per workspace (non-reactive handles),
  // constructed LAZILY on the first real editor need. `monaco-editor` (a multi-MB
  // dependency reached via `monaco-adapters`) is dynamically imported here so it
  // never lands on the cold-launch static chain — a workspace whose user only
  // opens terminals or agent chats never pays for it. `armEditor` is idempotent
  // and concurrency-safe (single shared construction + promise).
  let modelRegistry: ModelRegistry | undefined
  let editorManager: EditorManager | undefined
  let armPromise: Promise<void> | undefined

  const armEditor = (): Promise<void> => {
    if (editorManager) return Promise.resolve()
    if (armPromise) return armPromise
    // Loaded alongside monaco-adapters (not a static top-level import): a
    // static import here would cycle back through the window pane store's
    // own pane-slice, which resolves an editor manager BY workspace id via
    // this very module (workspace-store-registry.ts → workspace-store.ts).
    armPromise = Promise.all([
      import('@/features/editor/lib/monaco-adapters'),
      import('@/features/panes/stores/window-pane-store'),
    ])
      .then(([{ EDITOR_CREATE_OPTIONS, langForUri, realEditorApi, realModelApi }, { windowPaneStore }]) => {
        // `text(uri)` reads the buffer content for the file at that uri, or '' if
        // it isn't loaded; `lang(uri)` derives the Monaco language id from the path.
        // Buffers are window-level now (Task 26) — scope the lookup to THIS
        // workspace's own buffers (`buf.workspaceId`), since a hidden retained
        // workspace can hold a buffer at the same relative path.
        const registry = new ModelRegistry(realModelApi())
        const meta: BufferMeta = {
          lang: (uri) => langForUri(uri),
          text: (uri) => {
            const fsPath = uriToFsPath(uri)
            const buf = windowPaneStore
              .getState()
              .buffers.find(
                (b) => b.type === 'editor' && b.path === fsPath && b.workspaceId === wsId,
              )
            return buf && 'content' in buf ? buf.content : ''
          },
        }
        modelRegistry = registry
        editorManager = new EditorManager(realEditorApi(EDITOR_CREATE_OPTIONS), registry, meta)
      })
      .catch((err) => {
        // A transient chunk-load failure (offline / CDN hiccup) must not wedge the
        // seam permanently: drop the cached promise so a later EditorPane mount
        // retries the import instead of re-awaiting a forever-rejected promise.
        armPromise = undefined
        throw err
      })
    return armPromise
  }

  const activeEditorRegistry = createActiveEditorRegistry()

  // `editorManager`/`modelRegistry` are getters (not plain values) so their
  // post-arm assignment is observed by every reader through the SAME store object
  // — Object.assign would snapshot the (undefined) values at creation time.
  return Object.defineProperties(store, {
    modelRegistry: { get: () => modelRegistry, enumerable: true },
    editorManager: { get: () => editorManager, enumerable: true },
    armEditor: { value: armEditor, enumerable: true },
    activeEditorRegistry: { value: activeEditorRegistry, enumerable: true },
  }) as WorkspaceStore
}
