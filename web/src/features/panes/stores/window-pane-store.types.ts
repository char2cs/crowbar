// web/src/features/panes/stores/window-pane-store.types.ts
import type { PaneSlice } from './slices/pane-slice'
import type { BufferSlice } from './slices/buffer-slice'

/**
 * Task 26: `panes`/`buffers` used to be two of the many slices folded into
 * each PER-WORKSPACE `WorkspaceState` (`workspace-store.types.ts`) — one
 * whole store instance per workspace id, destroyed on eviction. That is the
 * root cause the model spec names: two panes holding chats from two
 * different workspaces could never live in the same layout, and evicting a
 * workspace killed a live pane arrangement outright.
 *
 * `WindowPaneState` is everything that moved out: ONE flat layout/buffer
 * list for the whole window, created once, never destroyed by workspace
 * lifecycle. `agentChats`/`lsp`/`terminal`/`fileWatcher`/`recentFiles`/
 * `branchReview` stay put on `WorkspaceState` — those genuinely are
 * per-workspace live resources (a socket, an LSP connection, a Monaco
 * model registry), not declarative layout.
 */
export type WindowPaneState = PaneSlice & BufferSlice
