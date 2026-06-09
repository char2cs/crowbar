const crowbar = (window as unknown as { __CROWBAR__?: { api?: string } }).__CROWBAR__

// Resolve a relative WS path (e.g. "/v0/ws/git?wsId=x") to an absolute ws://
// or wss:// URL. The browser WebSocket constructor rejects relative URLs, so
// stores keep dialing relative paths (matched by the mock layer and asserted in
// the contract tests) and the manager resolves them here at connect time.
// Precedence mirrors API_BASE: the Tauri bridge, then VITE_API_URL, then the
// page origin.
export function wsUrl(
  path: string,
): string {
  if (/^wss?:\/\//i.test(path)) return path

  const base = crowbar?.api || import.meta.env.VITE_API_URL || window.location.origin
  const resolved = new URL(path, base)
  if (resolved.protocol === 'https:') resolved.protocol = 'wss:'
  else if (resolved.protocol === 'http:') resolved.protocol = 'ws:'
  return resolved.toString()
}
