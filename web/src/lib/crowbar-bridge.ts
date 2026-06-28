// Crowbar system operations backed by the Go daemon's /v0 API.

import { Channel } from '@tauri-apps/api/core'

import { apiFetch } from '@/lib/api'
import { wsUrl } from '@/lib/ws/url'
import { workspaceBase } from '@/lib/workspace-scope-url'

// ── Terminal PTY ──────────────────────────────────────────────────────────────
// Each session is a WebSocket to the daemon's PTY handler. The wire protocol is
// JSON: server→client {sessionId, data}; client→server {data} for input and
// {type:'resize', cols, rows} for SIGWINCH.
//
// On the desktop app the browser WebSocket API can't reach the daemon (its only
// endpoint is the `crowbar://` unix-socket proxy, and `new WebSocket` rejects
// every scheme but ws/wss). There, Rust is the WebSocket client and bridges the
// PTY to the webview over a Tauri Channel — see the `isTauri()` branches below
// and desktop/src-tauri/src/terminal.rs. Both paths honour the same contract.

interface TerminalConnection {
  ws: WebSocket
  listener: ((data: string) => void) | null
  outputBuffer: string[]
  inputQueue: string[]
  open: boolean
}

const terminals = new Map<string, TerminalConnection>()

// Transport-drop notification: registered callbacks fire when the WS closes
// unexpectedly (e.g. daemon restart) while the entry is still in `terminals`.
// A clean terminalDetach removes the entry BEFORE calling ws.close(), so the
// `terminals.has(connectionId)` check correctly distinguishes unexpected drops
// from intentional detaches.
const dropCallbacks = new Map<string, Set<() => void>>()

// Desktop transport: output arrives over a Tauri Channel instead of a WebSocket.
// Same buffer-until-listener semantics as the browser TerminalConnection.
interface TauriTerminal {
  listener: ((data: string) => void) | null
  outputBuffer: string[]
}

const tauriTerminals = new Map<string, TauriTerminal>()

// §3: PTY routes are workspace-scoped now (.../workspaces/:w/terminals[/:id/ws]).
// terminalClose receives only the sessionId, so we record the hierarchical base
// per session at create time to build the DELETE/PTY-WS paths.
const sessionBases = new Map<string, string>()

// Wire a browser WebSocket for a connectionId into the `terminals` map.
// Extracted from terminalCreate so terminalAttach can reuse it without a POST.
function openBrowserSocket(connectionId: string, base: string): void {
  const ws = new WebSocket(wsUrl(`${base}/${encodeURIComponent(connectionId)}/ws`))
  const conn: TerminalConnection = { ws, listener: null, outputBuffer: [], inputQueue: [], open: false }
  ws.onopen = () => {
    conn.open = true
    for (const data of conn.inputQueue) ws.send(JSON.stringify({ data }))
    conn.inputQueue = []
  }
  ws.onmessage = (event) => {
    let data: string | undefined
    try { data = (JSON.parse(event.data as string) as { data?: string }).data } catch { return }
    if (typeof data !== 'string') return
    if (conn.listener) conn.listener(data)
    else conn.outputBuffer.push(data)
  }
  ws.onerror = () => {
    // Error events are followed by a close event on the same socket; all
    // cleanup is handled in the onclose handler below.
  }
  ws.onclose = () => {
    // Only treat the close as unexpected if the entry is still in `terminals`.
    // terminalDetach removes the entry BEFORE calling ws.close(), so a clean
    // detach never reaches the drop-notification branch.
    if (!terminals.has(connectionId)) return
    terminals.delete(connectionId)
    const cbs = dropCallbacks.get(connectionId)
    if (cbs) {
      for (const cb of cbs) cb()
    }
  }
  terminals.set(connectionId, conn)
}

// Wire a Tauri channel for a connectionId into the `tauriTerminals` map and ask
// Rust to open the WS. `terminal_open` REQUIRES an `onData: Channel<string>`
// (see desktop/src-tauri/src/terminal.rs) — omitting it makes the invoke reject.
// Extracted from terminalCreate so terminalAttach can reuse it without a POST.
async function openTauriSocket(connectionId: string, wsPath: string): Promise<void> {
  const conn: TauriTerminal = { listener: null, outputBuffer: [] }
  const channel = new Channel<string>()
  channel.onmessage = (data) => {
    if (conn.listener) conn.listener(data)
    else conn.outputBuffer.push(data)
  }
  tauriTerminals.set(connectionId, conn)
  await tauriInvoke('terminal_open', { sessionId: connectionId, wsPath, onData: channel })
}

// Create a PTY session in the workspace and open its stream. Returns the
// sessionId, which the terminal hooks use as the connection id. The project/repo
// are resolved from the active workspace route scope (workspaceBase).
export async function terminalCreate(wsId: string, profileId?: string): Promise<string> {
  const base = `${workspaceBase(wsId)}/terminals`
  const { sessionId } = await apiFetch<{ sessionId: string }>(base, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(profileId ? { profileId } : {}),
  })
  sessionBases.set(sessionId, base)

  // Desktop: hand a Channel to Rust, which opens the WS over the unix socket and
  // pumps PTY output back through it. Pass the hierarchical PTY path so Rust
  // dials the workspace-scoped route, not the removed flat one.
  if (isTauri()) {
    await openTauriSocket(sessionId, `${base}/${encodeURIComponent(sessionId)}/ws`)
    return sessionId
  }

  openBrowserSocket(sessionId, base)
  return sessionId
}

export async function terminalWrite(id: string, data: string): Promise<void> {
  if (isTauri()) {
    if (tauriTerminals.has(id)) await tauriInvoke('terminal_send', { sessionId: id, data })
    return
  }
  const conn = terminals.get(id)
  if (!conn) return
  if (conn.open) conn.ws.send(JSON.stringify({ data }))
  else conn.inputQueue.push(data)
}

export async function terminalResize(id: string, rows: number, cols: number): Promise<void> {
  if (isTauri()) {
    if (tauriTerminals.has(id)) await tauriInvoke('terminal_resize', { sessionId: id, rows, cols })
    return
  }
  const conn = terminals.get(id)
  if (conn?.open) conn.ws.send(JSON.stringify({ type: 'resize', cols, rows }))
}

export async function terminalClose(id: string): Promise<void> {
  // The DELETE is the hierarchical .../terminals/:sessionId under the workspace
  // base recorded at create time. If the base is unknown (e.g. a session created
  // before a reload), skip the REST call — the PTY is still torn down locally.
  const base = sessionBases.get(id)
  const deletePath = base ? `${base}/${encodeURIComponent(id)}` : null
  if (isTauri()) {
    if (tauriTerminals.delete(id)) await tauriInvoke('terminal_close', { sessionId: id })
    if (deletePath) await apiFetch(deletePath, { method: 'DELETE' }).catch(() => {})
    sessionBases.delete(id)
    return
  }
  const conn = terminals.get(id)
  if (conn) {
    conn.ws.close()
    terminals.delete(id)
  }
  if (deletePath) await apiFetch(deletePath, { method: 'DELETE' }).catch(() => {})
  sessionBases.delete(id)
}

// Register the output sink for a session, flushing any frames that arrived
// before the listener attached (e.g. the shell's first prompt).
export function terminalListen(id: string, onData: (data: string) => void): () => void {
  if (isTauri()) {
    const conn = tauriTerminals.get(id)
    if (!conn) return () => {}
    conn.listener = onData
    if (conn.outputBuffer.length > 0) {
      for (const chunk of conn.outputBuffer) onData(chunk)
      conn.outputBuffer = []
    }
    return () => {
      if (conn.listener === onData) conn.listener = null
    }
  }
  const conn = terminals.get(id)
  if (!conn) return () => {}
  conn.listener = onData
  if (conn.outputBuffer.length > 0) {
    for (const chunk of conn.outputBuffer) onData(chunk)
    conn.outputBuffer = []
  }
  return () => {
    if (conn.listener === onData) conn.listener = null
  }
}

// Detach the WS transport for a workspace switch: closes the socket (the daemon
// records a per-client detach and keeps the PTY running) WITHOUT issuing DELETE.
// `sessionBases` is intentionally retained so terminalAttach can re-dial later.
export async function terminalDetach(connectionId: string): Promise<void> {
  if (isTauri()) {
    if (tauriTerminals.delete(connectionId)) {
      await tauriInvoke('terminal_close', { sessionId: connectionId }).catch(() => {})
    }
    return
  }
  const conn = terminals.get(connectionId)
  if (conn) {
    conn.ws.close()
    terminals.delete(connectionId)
  }
}

// True when a live WS/channel transport exists for this connectionId. Used by the
// reconnect resolver: a surviving store entry whose transport was detached on a
// workspace switch must be RE-ATTACHED, not reused as-is.
export function terminalHasTransport(connectionId: string): boolean {
  return terminals.has(connectionId) || tauriTerminals.has(connectionId)
}

// Attach to an EXISTING daemon PTY (after a workspace switch) without creating a
// new one. The daemon replays its ring snapshot on attach, restoring scrollback.
export async function terminalAttach(connectionId: string, base: string): Promise<void> {
  sessionBases.set(connectionId, base)
  if (isTauri()) {
    await openTauriSocket(connectionId, `${base}/${encodeURIComponent(connectionId)}/ws`)
    return
  }
  openBrowserSocket(connectionId, base)
}

// List the daemon's live session connectionIds for a workspace. The `base` is
// `${workspaceBase(wsId)}/terminals`. Used by resolveTerminalConnection to
// confirm a persisted id is still alive before re-attaching.
export async function terminalListLive(base: string): Promise<string[]> {
  // Two response shapes exist: git workspaces return TerminalSessionDTO[] (objects
  // with id/status), while the home workspace endpoint returns a plain string[] of
  // session ids. Handle BOTH — mapping `.id` over a string[] yields [undefined]
  // (→ [null] on the wire), which silently broke home-workspace reconnect.
  const list = await apiFetch<Array<string | { id?: string; status?: string }>>(base)
  const ids: string[] = []
  for (const item of list) {
    if (typeof item === 'string') {
      ids.push(item)
    } else if (item && typeof item === 'object' && item.id && item.status !== 'ended') {
      ids.push(item.id)
    }
  }
  return ids
}

// Register a callback that fires when the browser-socket transport for
// `connectionId` drops unexpectedly (daemon restart, network loss). Returns
// an unsubscribe function. Multiple subscribers are supported but a single
// mounted terminal tab is the normal case.
//
// Tauri path: channel-drop detection is not yet wired on the Rust side.
// The browser path fires reliably on ws.onclose while the entry is mapped.
export function onTransportDrop(connectionId: string, cb: () => void): () => void {
  let cbs = dropCallbacks.get(connectionId)
  if (!cbs) {
    cbs = new Set()
    dropCallbacks.set(connectionId, cbs)
  }
  cbs.add(cb)
  return () => {
    const set = dropCallbacks.get(connectionId)
    if (!set) return
    set.delete(cb)
    if (set.size === 0) dropCallbacks.delete(connectionId)
  }
}

// Test-only: expose internal maps for unit tests. Do not use in app code.
export function __getBridgeInternals() {
  return { terminals, tauriTerminals, sessionBases, dropCallbacks }
}

// ── File Clipboard ────────────────────────────────────────────────────────────
// FUTURE: Go API file operations at /api/fs/clipboard

export interface ClipboardEntry {
  path: string
  is_dir: boolean
}

export interface FileClipboardState {
  entries: ClipboardEntry[]
  operation: 'copy' | 'cut'
}

export interface PastedEntry {
  path: string
  success: boolean
}

let _clipboard: FileClipboardState | null = null

export async function clipboardSet(
  entries: ClipboardEntry[],
  operation: 'copy' | 'cut',
): Promise<void> {
  _clipboard = { entries, operation }
  // FUTURE: POST /api/fs/clipboard/set
}

export async function clipboardPaste(_targetDirectory: string): Promise<PastedEntry[]> {
  // FUTURE: POST /api/fs/clipboard/paste
  return []
}

export async function clipboardGet(): Promise<FileClipboardState | null> {
  // FUTURE: GET /api/fs/clipboard
  return _clipboard
}

export async function clipboardClear(): Promise<void> {
  _clipboard = null
  // FUTURE: DELETE /api/fs/clipboard
}

// ── Native Dialogs ────────────────────────────────────────────────────────────
// FUTURE: Tauri plugin-dialog when crowbar desktop wrapper exposes it

export async function openDirectory(): Promise<string | null> {
  // FUTURE: @tauri-apps/plugin-dialog open({ directory: true, multiple: false })
  return null
}

// ── Window Management ─────────────────────────────────────────────────────────
// FUTURE: Tauri plugin calls when Crowbar's desktop wrapper exposes them

export async function setWindowTransparency(_enabled: boolean): Promise<void> {
  // FUTURE: invoke Tauri window transparency plugin
}

export async function setMacOSWindowAppearance(
  themeType: string,
  _transparencyEnabled: boolean,
): Promise<void> {
  // Pin the window-vibrancy NSVisualEffectView's appearance to the app theme so
  // the (dark) HUDWindow material renders a LIGHT frost in light mode. Targets the
  // blur view (NSWindow fallback), NOT the app-level NSApp.appearance that Tauri's
  // setTheme flips (fragile/inconsistent).
  if (!isTauri()) return
  await tauriInvoke('set_vibrancy_appearance', { dark: themeType === 'dark' })
}

export async function toggleMenuBar(_toggle: boolean): Promise<void> {
  // FUTURE: invoke Tauri menu bar plugin
}

// ── Tauri Helpers ─────────────────────────────────────────────────────────────

export function isTauri(): boolean {
  return typeof window !== 'undefined' && '__TAURI_INTERNALS__' in window
}

async function tauriInvoke(cmd: string, args?: Record<string, unknown>): Promise<void> {
  if (!isTauri()) throw new Error(`tauriInvoke called outside Tauri: ${cmd}`)
  // Use the global injected by Tauri before any JS runs — no npm import needed
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  await (window as any).__TAURI_INTERNALS__.invoke(cmd, args)
}

