import { readLocal, writeLocal } from '@/lib/local-storage'

const keyFor = (workspaceId: string) => `crowbar:terminal-reconnect:${workspaceId}`

function read(workspaceId: string): Record<string, string> {
  const raw = readLocal(keyFor(workspaceId))
  if (!raw) return {}
  try {
    return JSON.parse(raw) as Record<string, string>
  } catch {
    return {}
  }
}

function write(workspaceId: string, map: Record<string, string>): void {
  writeLocal(keyFor(workspaceId), JSON.stringify(map))
}

export function saveReconnect(
  workspaceId: string,
  tabSessionId: string,
  connectionId: string,
): void {
  const map = read(workspaceId)
  map[tabSessionId] = connectionId
  write(workspaceId, map)
}

export function loadReconnect(workspaceId: string, tabSessionId: string): string | null {
  return read(workspaceId)[tabSessionId] ?? null
}

export function clearReconnect(workspaceId: string, tabSessionId: string): void {
  const map = read(workspaceId)
  if (tabSessionId in map) {
    delete map[tabSessionId]
    write(workspaceId, map)
  }
}
