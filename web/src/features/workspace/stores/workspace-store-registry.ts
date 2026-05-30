import { createWorkspaceStore, type WorkspaceStore } from './workspace-store'
import { loadFromLocalStorage } from './workspace-persistence'
import { saveWorkspaceLayout } from '@/lib/persistence/workspace-layout'

const registry = new Map<string, WorkspaceStore>()
const persistTimers = new Map<string, ReturnType<typeof setTimeout>>()

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
          panes: [state.paneRoot],
          activePane: state.activePaneId,
          tabGroups: [state.bottomRoot],
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
  registry.delete(wsId)
}

export function getAllActiveWorkspaceIds(): string[] {
  return Array.from(registry.keys())
}
