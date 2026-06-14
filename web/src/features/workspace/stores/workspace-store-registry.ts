import { createWorkspaceStore, type WorkspaceStore } from './workspace-store'
import { loadFromLocalStorage } from './workspace-persistence'
import { saveWorkspaceLayout } from '@/lib/persistence/workspace-layout'
import { useHistoryStore } from '@/features/editor/stores/history-store'
import { cleanupBufferHistoryTracking } from '@/features/editor/stores/buffer-history-tracking'
import type { TerminalContent } from '@/features/panes/types/pane-content'

const registry = new Map<string, WorkspaceStore>()
const persistTimers = new Map<string, ReturnType<typeof setTimeout>>()

let _activeWorkspaceId: string | null = null

export function setActiveWorkspaceId(wsId: string): void {
  _activeWorkspaceId = wsId
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
