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

export function loadFromLocalStorage(wsId: string): WorkspaceSnapshot | null {
  try {
    const raw = localStorage.getItem(storageKey(wsId))
    if (!raw) return null
    return JSON.parse(raw) as WorkspaceSnapshot
  } catch {
    return null
  }
}
