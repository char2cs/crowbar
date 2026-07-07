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

// branchReview is a virtual/ephemeral pane: it is never
// serialized to the session and must not be restored from a stale snapshot —
// it would render an empty tab with no live review state. We drop it on
// restore (the simpler correct option: the entry points re-open it on demand).
const NON_RESTORABLE_BUFFER_TYPES = new Set(['branchReview'])

export function loadFromLocalStorage(wsId: string): WorkspaceSnapshot | null {
  try {
    const raw = localStorage.getItem(storageKey(wsId))
    if (!raw) return null
    const snapshot = JSON.parse(raw) as WorkspaceSnapshot
    // Strip non-restorable virtual buffers so they don't show as phantom tabs.
    if (snapshot.buffers) {
      snapshot.buffers = snapshot.buffers.filter(
        (b) => !NON_RESTORABLE_BUFFER_TYPES.has((b as { type: string }).type),
      )
    }
    return snapshot
  } catch {
    return null
  }
}
