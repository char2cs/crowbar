import type { WorkspaceSnapshot } from './workspace-store'
import { readLocal } from '@/lib/local-storage'

function storageKey(wsId: string): string {
  return `workspace:${wsId}:state`
}

export function loadFromLocalStorage(wsId: string): WorkspaceSnapshot | null {
  const raw = readLocal(storageKey(wsId))
  if (!raw) return null
  try {
    return JSON.parse(raw) as WorkspaceSnapshot
  } catch {
    return null
  }
}
