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

// Whether the configured API base can carry a WebSocket. Only http(s)/ws(s)
// bases can — a custom transport (e.g. the desktop's `crowbar://` unix-socket
// scheme, which is proxied over plain HTTP only) cannot, and constructing a
// `new WebSocket('crowbar://…')` throws "The string did not match the expected
// pattern". Callers must skip live channels when this returns false; live-push
// features degrade to no streaming (HTTP requests still work via the proxy)
// until a native event bridge exists.
export function isWebSocketCapable(): boolean {
  const base = crowbar?.api || import.meta.env.VITE_API_URL || window.location.origin
  return /^(https?|wss?):\/\//i.test(base)
}
