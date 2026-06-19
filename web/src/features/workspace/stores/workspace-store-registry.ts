import { createWorkspaceStore, type WorkspaceStore } from './workspace-store'
import { loadFromLocalStorage } from './workspace-persistence'
import { saveWorkspaceLayout } from '@/lib/persistence/workspace-layout'
import { useHistoryStore } from '@/features/editor/stores/history-store'
import { cleanupBufferHistoryTracking } from '@/features/editor/stores/buffer-history-tracking'
import type { TerminalContent } from '@/features/panes/types/pane-content'
import { setActiveScopeWorkspaceId } from '@/lib/workspace-scope'

const registry = new Map<string, WorkspaceStore>()
const persistTimers = new Map<string, ReturnType<typeof setTimeout>>()

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
export {
  setWorkspaceScope,
  getWorkspaceScope,
  type WorkspaceScope,
} from '@/lib/workspace-scope'

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
    const snapshot = loadFromLocalStorage(wsId)
    const store = createWorkspaceStore(wsId, snapshot === null ? undefined : snapshot)

    store.subscribe((state) => {
      const existing = persistTimers.get(wsId)
      if (existing !== undefined) clearTimeout(existing)
      const timer = setTimeout(() => {
        persistTimers.delete(wsId)
        saveWorkspaceLayout({
          workspaceId: wsId,
          panes: state.panes,
          rootLayout: state.rootLayout,
          bottomLayout: state.bottomLayout,
          activePaneId: state.activePaneId,
          mostRecentActivePaneIds: state.mostRecentActivePaneIds,
          buffers: state.buffers,
          sidebarWidth: 0,
          rightSidebarWidth: 0,
          updatedAt: Date.now(),
        })
      }, 300)
      persistTimers.set(wsId, timer)
    })

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

  const store = registry.get(wsId)
  if (store) {
    const { buffers } = store.getState()

    // Kill terminal PTY sessions
    const terminalBuffers = buffers.filter((b) => b.type === 'terminal')
    if (terminalBuffers.length > 0) {
      void import('@/features/terminal/lib/kill-terminal-session').then(({ killTerminalSession }) => {
        for (const buf of terminalBuffers) {
          void killTerminalSession((buf as TerminalContent).sessionId).catch(() => {})
        }
      })
    }

    // Cleanup undo tracker and history for each buffer
    for (const buf of buffers) {
      cleanupBufferHistoryTracking(buf.id)
      useHistoryStore.getState().actions.clearHistory(buf.id)
    }

    // Dispose editor resources
    store.editorManager.disposeAll()
  }

  registry.delete(wsId)
}

export function getAllActiveWorkspaceIds(): string[] {
  return Array.from(registry.keys())
}
