// web/src/features/panes/stores/window-pane-store.ts
import { createStore, type StoreApi } from 'zustand'
import { immer } from 'zustand/middleware/immer'
import type { WindowPaneState } from './window-pane-store.types'
import { createPaneSlice } from './slices/pane-slice'
import { createBufferSlice } from './slices/buffer-slice'
import { saveWorkspaceLayout } from '@/lib/persistence/workspace-layout'
import { stripNewTabs } from '@/features/panes/utils/persisted-layout'
import { saveSessionToStore } from '@/features/editor/stores/buffer-session-persistence'

export type WindowPaneStore = StoreApi<WindowPaneState>

export type WindowPaneSnapshot = Partial<
  Pick<
    WindowPaneState,
    | 'panes'
    | 'rootLayout'
    | 'bottomLayout'
    | 'activePaneId'
    | 'fullscreenPaneId'
    | 'mostRecentActivePaneIds'
    | 'dormantArrangements'
    | 'buffers'
  >
>

/**
 * Task 26: one pane/buffer store for the whole window — created once, never
 * destroyed on workspace switch or eviction (the trap the model spec names:
 * the OLD per-workspace registry's `destroyWorkspaceStore` used to kill a
 * live pane layout the moment its workspace aged out of keep-alive, or the
 * user left to project home). A `PaneGroup`'s own chat resolves which
 * workspace it belongs to (via the chat's own `workspaceId`, looked up
 * through the owning workspace store — see `pane-slice.ts`'s `isChatWorking`
 * use and `workspace-store-registry.ts`), never by which store instance
 * happens to hold it.
 *
 * `createWindowPaneStore` is exported (not just the singleton below) so
 * tests can exercise a fresh, isolated store per test — see
 * `window-pane-store.test.ts`.
 */
export function createWindowPaneStore(snapshot?: WindowPaneSnapshot): WindowPaneStore {
  const store = createStore<WindowPaneState>()(
    immer((set, get, api): WindowPaneState => ({
      ...createPaneSlice(set, get, api),
      ...createBufferSlice(set, get, api),
      ...(snapshot ?? {}),
    })),
  )

  // Debounced persistence, re-keyed to the one window/session row (was
  // per-workspace on the old registry — see workspace-layout.ts). Same
  // shallow-compare-then-debounce shape as the old registry subscription:
  // skip the (frequent) non-persisted mutations immediately, without arming
  // the timer.
  let persistTimer: ReturnType<typeof setTimeout> | undefined
  store.subscribe((state, prev) => {
    if (
      state.panes === prev.panes &&
      state.rootLayout === prev.rootLayout &&
      state.bottomLayout === prev.bottomLayout &&
      state.activePaneId === prev.activePaneId &&
      state.mostRecentActivePaneIds === prev.mostRecentActivePaneIds &&
      state.buffers === prev.buffers
    ) {
      return
    }

    if (persistTimer !== undefined) clearTimeout(persistTimer)
    persistTimer = setTimeout(() => {
      persistTimer = undefined
      const current = store.getState()
      const persistable = stripNewTabs({ buffers: current.buffers, panes: current.panes })
      saveWorkspaceLayout({
        // WINDOW_SESSION_ID is stamped in workspace-layout.ts's own
        // saveWorkspaceLayout — this workspaceId is overwritten there.
        workspaceId: '',
        panes: persistable.panes,
        rootLayout: current.rootLayout,
        bottomLayout: current.bottomLayout,
        activePaneId: current.activePaneId,
        mostRecentActivePaneIds: current.mostRecentActivePaneIds,
        buffers: persistable.buffers,
        sidebarWidth: 0,
        rightSidebarWidth: 0,
        updatedAt: Date.now(),
      })
    }, 300)
  })

  // Persist session (open buffers + active buffer) to IndexedDB on buffer
  // changes — moved verbatim from the old per-workspace `createWorkspaceStore`
  // (see git history), now firing once for the window's one flat buffer list
  // instead of once per retained workspace store.
  store.subscribe((state, prev) => {
    if (state.buffers === prev.buffers) return
    const activePane = state.panes[state.activePaneId] ?? null
    saveSessionToStore(state.buffers, activePane?.activeEditorTabId ?? null)
  })

  return store
}

/** The one window-level pane/buffer store, created once at module load and
 *  never destroyed. Every consumer that used to read panes/buffers off the
 *  ambient per-workspace `useWorkspaceStoreContext()` now reads this instead
 *  — see `use-pane-store.ts` / `use-buffer-store.ts`. */
export const windowPaneStore = createWindowPaneStore()

/**
 * Test-only reset. `target.setState(createWindowPaneStore().getState())`
 * looks like a valid "reset to defaults" but is NOT: it also overwrites
 * `target`'s own `paneActions`/`bufferActions` with a second, throwaway
 * store's closures — which are bound to THAT store's own internal `set`/
 * `get`, not `target`'s. Every later `target.getState().paneActions.foo()`
 * call then silently mutates the discarded throwaway store, leaving `target`
 * itself frozen at whatever data this reset assigned, forever. This resets
 * only the plain-data fields and leaves `target`'s actions untouched.
 */
export function resetWindowPaneStoreForTests(target: WindowPaneStore = windowPaneStore): void {
  const fresh = createWindowPaneStore().getState()
  target.setState({
    panes: fresh.panes,
    rootLayout: fresh.rootLayout,
    bottomLayout: fresh.bottomLayout,
    activePaneId: fresh.activePaneId,
    fullscreenPaneId: fresh.fullscreenPaneId,
    mostRecentActivePaneIds: fresh.mostRecentActivePaneIds,
    dormantArrangements: fresh.dormantArrangements,
    buffers: fresh.buffers,
    closedBuffersHistory: fresh.closedBuffersHistory,
    pendingClose: fresh.pendingClose,
    maxOpenTabs: fresh.maxOpenTabs,
  })
}
