import type { WorkspaceSnapshot } from './workspace-store'

function storageKey(wsId: string): string {
  return `workspace:${wsId}:state`
}

export function saveToLocalStorage(wsId: string, snapshot: WorkspaceSnapshot): void {
  try {
    localStorage.setItem(storageKey(wsId), JSON.stringify(snapshot))
  } catch {
    // localStorage full or unavailable — silently skip
  }
}

const REMOVED_BUFFER_TYPES = new Set(['branchReview'])

export function loadFromLocalStorage(wsId: string): WorkspaceSnapshot | null {
  try {
    const raw = localStorage.getItem(storageKey(wsId))
    if (!raw) return null
    const snapshot = JSON.parse(raw) as WorkspaceSnapshot
    // Strip buffers whose types no longer exist (e.g. branchReview was deleted).
    // Without this, stale buffers with isUncloseable:true show as phantom uncloseable tabs.
    if (snapshot.buffers) {
      snapshot.buffers = snapshot.buffers.filter(
        (b) => !REMOVED_BUFFER_TYPES.has((b as { type: string }).type),
      )
    }
    return snapshot
  } catch {
    return null
  }
}
