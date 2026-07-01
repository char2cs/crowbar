// Backstop mapping of frontend tab id -> daemon connectionId, per workspace.
// In-memory useTerminalStore is the primary holder across a workspace switch;
// this localStorage mirror survives a webview reload while the daemon stays up.
const keyFor = (workspaceId: string) => `crowbar:terminal-reconnect:${workspaceId}`

function read(workspaceId: string): Record<string, string> {
  try {
    const raw = localStorage.getItem(keyFor(workspaceId))
    return raw ? (JSON.parse(raw) as Record<string, string>) : {}
  } catch {
    return {}
  }
}

function write(workspaceId: string, map: Record<string, string>): void {
  try {
    localStorage.setItem(keyFor(workspaceId), JSON.stringify(map))
  } catch {
    /* quota / private mode — best effort */
  }
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
