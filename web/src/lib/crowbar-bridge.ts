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

// One parsed daemon→client terminal frame. `snapshot` marks a self-contained
// ground-state redraw (the daemon's serialized screen model) that must be
// applied onto a RESET xterm buffer — the attach redraw and the post-resize
// resync — as opposed to incremental PTY output that appends.
export interface TerminalFrame {
  data: string
  snapshot: boolean
}

// parseTerminalFrame decodes one wire frame ({sessionId, data, snapshot?})
// shared by both transports (browser WebSocket text frames and the whole-frame
// strings Rust forwards down the Tauri channel). Returns null for malformed
// frames.
function parseTerminalFrame(raw: string): TerminalFrame | null {
  try {
    const msg = JSON.parse(raw) as { data?: unknown; snapshot?: unknown }
    if (typeof msg.data !== 'string') return null
    return { data: msg.data, snapshot: msg.snapshot === true }
  } catch {
    return null
  }
}

interface TerminalConnection {
  ws: WebSocket
  listener: ((frame: TerminalFrame) => void) | null
  outputBuffer: TerminalFrame[]
  inputQueue: string[]
  // The most recent theme frame pushed before the socket opened. Unlike input, only the
  // LAST theme matters, so it coalesces to one frame flushed on open — this is what makes
  // the initial on-attach theme push (which races the WS handshake) reach the daemon, so a
  // freshly started app detects the right background instead of the default.
  pendingTheme: string | null
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
  listener: ((frame: TerminalFrame) => void) | null
  outputBuffer: TerminalFrame[]
  unlisten?: () => void // unsubscribe fn for the terminal:transport-dropped listener
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
  const conn: TerminalConnection = {
    ws,
    listener: null,
    outputBuffer: [],
    inputQueue: [],
    pendingTheme: null,
    open: false,
  }
  ws.onopen = () => {
    conn.open = true
    for (const data of conn.inputQueue) ws.send(JSON.stringify({ data }))
    conn.inputQueue = []
    if (conn.pendingTheme) {
      ws.send(conn.pendingTheme)
      conn.pendingTheme = null
    }
  }
  ws.onmessage = (event) => {
    const frame = parseTerminalFrame(event.data as string)
    if (!frame) return
    if (conn.listener) conn.listener(frame)
    else conn.outputBuffer.push(frame)
  }
  ws.onerror = () => {
    // Error events are followed by a close event on the same socket; all
    // cleanup is handled in the onclose handler below.
  }
  ws.onclose = () => {
    // Only treat the close as unexpected if THIS connection is still the one
    // registered. terminalDetach removes the entry BEFORE calling ws.close(), so a
    // clean detach never reaches the drop-notification branch.
    //
    // Identity, not presence: an attach-only terminal detaches on unmount and
    // re-attaches on the next mount (a chat tab switch — and every mount under
    // StrictMode), so a NEW connection can be registered under the same
    // connectionId while the old socket's close is still in flight. A `has()` check
    // cannot tell the two apart, and would let the dead socket delete the live
    // entry and fire a spurious transport-drop against it.
    if (terminals.get(connectionId) !== conn) return
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
  channel.onmessage = (raw) => {
    // Rust forwards the wire frame whole; parse it here so both transports
    // share one frame decoder.
    const frame = parseTerminalFrame(raw)
    if (!frame) return
    if (conn.listener) conn.listener(frame)
    else conn.outputBuffer.push(frame)
  }
  tauriTerminals.set(connectionId, conn)

  // Mirror openBrowserSocket's ws.onclose semantics for the Tauri path: subscribe
  // to `terminal:transport-dropped` events emitted by Rust after the reader loop
  // exits. Only treat the event as an unexpected drop when the entry is still in
  // `tauriTerminals` — a clean terminalClose/terminalDetach deletes the entry
  // BEFORE invoking terminal_close, so the guard sees has()===false and no-ops.
  const { listen } = await import('@tauri-apps/api/event')
  const unlisten = await listen<string>('terminal:transport-dropped', (event) => {
    if (event.payload !== connectionId) return
    // Identity, not presence — same reason as openBrowserSocket's onclose: a
    // detach→re-attach cycle on one connectionId (chat tab switch, StrictMode
    // remount) can register a fresh entry before the dead one's drop event lands.
    if (tauriTerminals.get(connectionId) !== conn) return
    tauriTerminals.delete(connectionId)
    // Unsubscribe on the way out. Deleting the entry above is what makes every later
    // firing a no-op, so the listener is dead weight from here on — but it is dead
    // weight held in Rust's listener registry, and terminalClose/terminalDetach can no
    // longer reach it (they only unlisten while the entry still exists). Every daemon
    // restart drops all sessions and re-attaches them, so without this the registry
    // grows by one listener per terminal per restart, for the life of the app.
    conn.unlisten?.()
    const cbs = dropCallbacks.get(connectionId)
    if (cbs) {
      for (const cb of cbs) cb()
    }
  })
  conn.unlisten = unlisten

  try {
    await tauriInvoke('terminal_open', { sessionId: connectionId, wsPath, onData: channel })
  } catch (err) {
    // terminal_open rejected: the map entry and the transport-dropped listener
    // were registered up-front (so buffered output isn't lost in the race window).
    // On failure they must be torn down, or a phantom tauriTerminals entry lingers
    // — terminalHasTransport would report a live transport that never opened, and a
    // later attach/create would reuse the dead entry instead of re-dialing.
    tauriTerminals.delete(connectionId)
    unlisten()
    throw err
  }
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
    // A send can race the transport being retired underneath us. When Rust detects a
    // broken/half-open socket it retires the session and emits `terminal:transport-dropped`
    // (Phase 0); a `terminal_send` that lands in the window between that retirement and the
    // drop event deleting our map entry rejects with "no open terminal session". Swallow it
    // — the drop event drives the re-attach — so a transient rejection never propagates into
    // the write buffer (writeChunk awaits this). Best-effort, matching terminalResize.
    if (tauriTerminals.has(id))
      await tauriInvoke('terminal_send', { sessionId: id, data }).catch(() => {})
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

// Ask the daemon to re-emit its model snapshot for this session (post-resize
// convergence). The daemon no-ops at an idle shell prompt; when a foreground
// app is running, a snapshot frame arrives on the output stream and the
// terminal hook resets the local buffer with it.
export async function terminalResync(id: string): Promise<void> {
  if (isTauri()) {
    if (tauriTerminals.has(id)) await tauriInvoke('terminal_resync', { sessionId: id })
    return
  }
  const conn = terminals.get(id)
  if (conn?.open) conn.ws.send(JSON.stringify({ type: 'resync' }))
}

// Push the host terminal's light/dark theme to the daemon so a foreground app's automatic
// theme can follow a Crowbar theme switch: `bg`/`fg` are the resolved default colours (an
// OSC 11/10 query answers with them) and `dark` is the light/dark polarity for the daemon's
// DEC 2031 CSI ?997;n theme-change report. Best-effort and idempotent — the daemon updates the
// query colours every call and dedupes the notification by polarity.
export async function terminalSetTheme(
  id: string,
  theme: { background: string; foreground: string; dark: boolean },
): Promise<void> {
  if (isTauri()) {
    if (tauriTerminals.has(id)) {
      await tauriInvoke('terminal_set_theme', {
        sessionId: id,
        bg: theme.background,
        fg: theme.foreground,
        dark: theme.dark,
      })
    }
    return
  }
  const conn = terminals.get(id)
  if (!conn) return
  const frame = JSON.stringify({
    type: 'theme',
    bg: theme.background,
    fg: theme.foreground,
    dark: theme.dark,
  })
  // Coalesce-until-open: the on-attach push can beat the WS handshake, and unlike input a
  // dropped theme frame would never be retried (there is no theme equivalent of a follow-up
  // keystroke), leaving the daemon on its default background.
  if (conn.open) conn.ws.send(frame)
  else conn.pendingTheme = frame
}

export async function terminalClose(id: string): Promise<void> {
  // The DELETE is the hierarchical .../terminals/:sessionId under the workspace
  // base recorded at create time. If the base is unknown (e.g. a session created
  // before a reload), skip the REST call — the PTY is still torn down locally.
  const base = sessionBases.get(id)
  const deletePath = base ? `${base}/${encodeURIComponent(id)}` : null
  if (isTauri()) {
    const tconn = tauriTerminals.get(id)
    if (tconn) {
      tauriTerminals.delete(id)
      tconn.unlisten?.()
      await tauriInvoke('terminal_close', { sessionId: id })
    }
    if (deletePath) await apiFetch(deletePath, { method: 'DELETE' }).catch(() => {})
    sessionBases.delete(id)
    return
  }
  const conn = terminals.get(id)
  if (conn) {
    terminals.delete(id)
    conn.ws.close()
  }
  if (deletePath) await apiFetch(deletePath, { method: 'DELETE' }).catch(() => {})
  sessionBases.delete(id)
}

// Register the output sink for a session, flushing any frames that arrived
// before the listener attached (e.g. the shell's first prompt / the attach
// snapshot). The listener receives parsed TerminalFrames — check `snapshot`
// to distinguish reset-and-redraw frames from incremental output.
export function terminalListen(id: string, onFrame: (frame: TerminalFrame) => void): () => void {
  if (isTauri()) {
    const conn = tauriTerminals.get(id)
    if (!conn) return () => {}
    conn.listener = onFrame
    if (conn.outputBuffer.length > 0) {
      for (const frame of conn.outputBuffer) onFrame(frame)
      conn.outputBuffer = []
    }
    return () => {
      if (conn.listener === onFrame) conn.listener = null
    }
  }
  const conn = terminals.get(id)
  if (!conn) return () => {}
  conn.listener = onFrame
  if (conn.outputBuffer.length > 0) {
    for (const frame of conn.outputBuffer) onFrame(frame)
    conn.outputBuffer = []
  }
  return () => {
    if (conn.listener === onFrame) conn.listener = null
  }
}

// Detach the WS transport for a workspace switch: closes the socket (the daemon
// records a per-client detach and keeps the PTY running) WITHOUT issuing DELETE.
// `sessionBases` is intentionally retained so terminalAttach can re-dial later.
export async function terminalDetach(connectionId: string): Promise<void> {
  if (isTauri()) {
    const tconn = tauriTerminals.get(connectionId)
    if (tconn) {
      tauriTerminals.delete(connectionId)
      tconn.unlisten?.()
      await tauriInvoke('terminal_close', { sessionId: connectionId }).catch(() => {})
    }
    return
  }
  const conn = terminals.get(connectionId)
  if (conn) {
    terminals.delete(connectionId)
    conn.ws.close()
  }
}

// True when a live WS/channel transport exists for this connectionId. Used by the
// reconnect resolver: a surviving store entry whose transport was detached on a
// workspace switch must be RE-ATTACHED, not reused as-is.
//
// This is LIVENESS, not just "we once created this": map presence is now truthful
// because a dead transport is actively removed from the map. The browser path deletes
// its entry on ws.onclose; the Tauri path deletes its entry when Rust emits
// `terminal:transport-dropped`, which Phase 0 makes fire for EVERY way a socket dies —
// a daemon close, a writer-side send failure, AND a silent half-open socket (read-idle
// timeout). So a session that is not provably alive is no longer left in the map for the
// resolver to reuse as a corpse: it is gone, `has()` is false, and the resolver re-attaches.
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
// Tauri path: channel-drop is wired — Rust emits `terminal:transport-dropped`
// after its reader loop exits (commit 8d47530); openTauriChannel subscribes and
// fires the registered callbacks. The browser path fires on ws.onclose instead.
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

// ── File Manager ──────────────────────────────────────────────────────────────

// The `reveal_in_finder` invoke should always settle in well under a second —
// see desktop/src-tauri/src/lib.rs for the (now-fixed) main-thread-blocking bug
// that could make it hang indefinitely. This guard is defence in depth: if a
// future regression (or a denied/reworked capability) makes the invoke hang
// again, callers of revealItemInFinder — which all `.catch` to surface a toast
// (see use-workspace-effects.ts) — get a rejection instead of silence.
const REVEAL_IN_FINDER_TIMEOUT_MS = 3_000

/** Reveal a file or directory in the OS file manager (Finder on macOS) with the
 *  item selected. `path` must be absolute. No-op outside Tauri (browser dev). */
export async function revealItemInFinder(path: string): Promise<void> {
  if (!isTauri()) return
  let timer: ReturnType<typeof setTimeout> | undefined
  const timeout = new Promise<never>((_, reject) => {
    timer = setTimeout(
      () => reject(new Error('reveal_in_finder timed out')),
      REVEAL_IN_FINDER_TIMEOUT_MS,
    )
  })
  try {
    await Promise.race([tauriInvoke('reveal_in_finder', { path }), timeout])
  } finally {
    clearTimeout(timer)
  }
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
