import { isTauri } from '@/lib/crowbar-bridge'

const crowbar = (window as unknown as { __CROWBAR__?: { api?: string } }).__CROWBAR__

// Resolve a relative WS path (e.g. "/v0/ws/git?wsId=x") to an absolute ws://
// or wss:// URL. The browser WebSocket constructor rejects relative URLs, so
// stores keep dialing relative paths (matched by the mock layer and asserted in
// the contract tests) and the manager resolves them here at connect time.
// Precedence mirrors API_BASE: the Tauri bridge, then VITE_API_URL, then the
// page origin.
export function wsUrl(path: string): string {
  if (/^wss?:\/\//i.test(path)) return path

  const base = crowbar?.api || import.meta.env.VITE_API_URL || window.location.origin
  const resolved = new URL(path, base)
  if (resolved.protocol === 'https:') resolved.protocol = 'wss:'
  else if (resolved.protocol === 'http:') resolved.protocol = 'ws:'
  return resolved.toString()
}

// Whether live WebSocket streaming is available for the active transport.
//
// On desktop the API base is the `crowbar://` unix-socket scheme, which the
// browser `WebSocket` constructor rejects — but the native `ws_bridge` Rust
// command bridges an arbitrary `/v0/...` WS route over the unix socket and the
// `TauriWebSocket` shim presents it as a WebSocket, so streaming IS available
// there. In the browser, only http(s)/ws(s) bases can carry a native WebSocket.
// When this returns false, callers skip live channels and live-push features
// degrade to no streaming (HTTP requests still work via the proxy).
export function isWebSocketCapable(): boolean {
  if (isTauri()) return true
  const base = crowbar?.api || import.meta.env.VITE_API_URL || window.location.origin
  return /^(https?|wss?):\/\//i.test(base)
}
