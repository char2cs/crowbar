import { createWorkspaceStore, type WorkspaceStore } from './workspace-store'
import { loadFromLocalStorage } from './workspace-persistence'
import { stripNewTabs } from './persisted-layout'
import { saveWorkspaceLayout } from '@/lib/persistence/workspace-layout'
import { useHistoryStore } from '@/features/editor/stores/history-store'
import { cleanupBufferHistoryTracking } from '@/features/editor/stores/buffer-history-tracking'
import type { TerminalContent } from '@/features/panes/types/pane-content'
import { isEditorContent } from '@/features/panes/types/pane-content'
import { setActiveScopeWorkspaceId } from '@/lib/workspace-scope'
import { clearWorkspaceFreshness } from '../lib/activation-freshness'

const registry = new Map<string, WorkspaceStore>()
const persistTimers = new Map<string, ReturnType<typeof setTimeout>>()
/** Unsubscribe functions for the per-workspace persistence subscriptions. */
const persistUnsubs = new Map<string, () => void>()

let _activeWorkspaceId: string | null = null

// §3/§7: workspace-scoped API/WS URLs are now hierarchical
// (/v0/projects/:p/repos/:r/workspaces/:w/...). The owning project+repo of the
// active workspace are threaded from the TanStack route and recorded so the many
// wsId-keyed callers (files, git, terminal, editor) can resolve the full scope
// without every signature growing two params. The scope MAP itself lives in the
// dependency-free `@/lib/workspace-scope` module so those lightweight builders
// don't import this heavy registry (which pulls in the editor/Monaco graph and
// timed out their dynamic-import unit tests). We re-export the setter/getter here
// for callers that already depend on the registry.
export { setWorkspaceScope, getWorkspaceScope, type WorkspaceScope } from '@/lib/workspace-scope'

export function setActiveWorkspaceId(wsId: string): void {
  _activeWorkspaceId = wsId
  setActiveScopeWorkspaceId(wsId)
}

export function getActiveWorkspaceStore(): WorkspaceStore | null {
  if (!_activeWorkspaceId) return null
  return registry.get(_activeWorkspaceId) ?? null
}

export function getActiveWorkspaceId(): string | null {
  return _activeWorkspaceId
}

export function getOrCreateWorkspaceStore(wsId: string): WorkspaceStore {
  if (!registry.has(wsId)) {
    // Stripped on the way IN as well as on the way out: the save path can only
    // filter what this build knows about, and a layout written by an older
    // build is exactly the case that matters — a tab whose content type has
    // since been retired restores into a renderer that no longer exists.
    const saved = loadFromLocalStorage(wsId)
    const snapshot =
      saved && saved.buffers && saved.panes
        ? { ...saved, ...stripNewTabs({ buffers: saved.buffers, panes: saved.panes }) }
        : (saved ?? undefined)
    const store = createWorkspaceStore(wsId, snapshot)

    // Subscribe to store changes and debounce persistence writes.
    // The callback runs a shallow-compare of the five persisted layout fields
    // before doing any work, so non-persisted mutations (LSP diagnostics,
    // terminal output, editor cursor, etc.) are ignored immediately without
    // arming the debounce timer.
    const unsub = store.subscribe((state, prev) => {
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

      const existing = persistTimers.get(wsId)
      if (existing !== undefined) clearTimeout(existing)
      const timer = setTimeout(() => {
        persistTimers.delete(wsId)
        const persistable = stripNewTabs({ buffers: state.buffers, panes: state.panes })
        saveWorkspaceLayout({
          workspaceId: wsId,
          panes: persistable.panes,
          rootLayout: state.rootLayout,
          bottomLayout: state.bottomLayout,
          activePaneId: state.activePaneId,
          mostRecentActivePaneIds: state.mostRecentActivePaneIds,
          buffers: persistable.buffers,
          sidebarWidth: 0,
          rightSidebarWidth: 0,
          updatedAt: Date.now(),
        })
      }, 300)
      persistTimers.set(wsId, timer)
    })
    persistUnsubs.set(wsId, unsub)

    registry.set(wsId, store)
  }
  return registry.get(wsId)!
}

export function destroyWorkspaceStore(wsId: string): void {
  const existing = persistTimers.get(wsId)
  if (existing !== undefined) {
    clearTimeout(existing)
    persistTimers.delete(wsId)
  }

  const unsub = persistUnsubs.get(wsId)
  if (unsub !== undefined) {
    unsub()
    persistUnsubs.delete(wsId)
  }

  const store = registry.get(wsId)
  if (store) {
    const { buffers } = store.getState()

    // Detach (not kill) pane terminal PTY sessions on workspace switch.
    // The PTY stays alive in the daemon; the WS transport is closed and the
    // connectionId is persisted to localStorage so re-entry can re-attach with
    // scrollback replay. killTerminalSession is still used on real tab close.
    const terminalBuffers = buffers.filter((b) => b.type === 'terminal')
    if (terminalBuffers.length > 0) {
      void import('@/features/terminal/lib/detach-terminal-session').then(
        ({ detachTerminalSession }) => {
          for (const buf of terminalBuffers) {
            void detachTerminalSession(wsId, (buf as TerminalContent).sessionId).catch(() => {})
          }
        },
      )
    }

    // Free cached git-blame for this workspace's open files. The blame store is a
    // global singleton keyed by file path, so clearAllBlame() would wipe blame for
    // OTHER still-active workspaces; we instead clear only this workspace's editor
    // buffer paths. Dynamic import avoids a registry → git-feature cycle, mirroring
    // the terminal branch above.
    const editorPaths: string[] = []
    for (const b of buffers) {
      if (isEditorContent(b)) editorPaths.push(b.path)
    }
    if (editorPaths.length > 0) {
      void import('@/features/git/stores/git-blame-store').then(({ useGitBlameStore }) => {
        const { clearBlameForFile } = useGitBlameStore.getState()
        for (const path of editorPaths) {
          clearBlameForFile(path)
        }
      })
    }

    // Cleanup undo tracker and history for each buffer
    for (const buf of buffers) {
      cleanupBufferHistoryTracking(buf.id)
      useHistoryStore.getState().actions.clearHistory(buf.id)
    }

    // Tear down the store's internal session-persistence subscription so a late
    // setState after teardown can't write this (now stale) workspace's session
    // to IndexedDB. Distinct from the registry's layout-persistence unsubscribe
    // handled above; this one lives inside the store itself.
    store._disposeSession()

    // Dispose editor resources (only if the workspace ever armed the editor —
    // a terminal/agent-only workspace never constructs the manager).
    store.editorManager?.disposeAll()
  }

  // Drop the warm-reactivation freshness ledger for this workspace so a future
  // workspace reusing the id can't inherit a stale "hidden briefly" stamp.
  clearWorkspaceFreshness(wsId)

  registry.delete(wsId)
}

export function getAllActiveWorkspaceIds(): string[] {
  return Array.from(registry.keys())
}
