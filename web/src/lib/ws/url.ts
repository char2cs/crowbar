import { isTauri } from '@/lib/crowbar-bridge'
import { apiBaseFor } from '@/lib/api/fetch'

const crowbar = (window as unknown as { __CROWBAR__?: { api?: string } }).__CROWBAR__

// Resolve a relative WS path ("/v0/ws/git?wsId=x") to an absolute ws(s):// URL at connect time,
// since WebSocket rejects relative URLs. Precedence mirrors API_BASE (Tauri bridge, VITE_API_URL,
// page origin); a non-'local' hostId resolves through apiBaseFor, as apiFetch does.
export function wsUrl(path: string, hostId = 'local'): string {
  if (/^wss?:\/\//i.test(path)) return path

  const base =
    hostId === 'local'
      ? crowbar?.api || import.meta.env.VITE_API_URL || window.location.origin
      : apiBaseFor(hostId)
  const resolved = new URL(path, base)
  if (resolved.protocol === 'https:') resolved.protocol = 'wss:'
  else if (resolved.protocol === 'http:') resolved.protocol = 'ws:'
  return resolved.toString()
}

// Whether live streaming is available: on desktop ws_bridge carries any `/v0/...` WS route over
// the unix socket via TauriWebSocket; in the browser only http(s)/ws(s) bases can. When false,
// callers skip live channels and features degrade to HTTP only.
export function isWebSocketCapable(): boolean {
  if (isTauri()) return true
  const base = crowbar?.api || import.meta.env.VITE_API_URL || window.location.origin
  return /^(https?|wss?):\/\//i.test(base)
}
