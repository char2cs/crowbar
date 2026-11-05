// Crowbar system operations backed by the Go daemon's /v0 API.

import { convertFileSrc as tauriConvertFileSrc } from '@tauri-apps/api/core'
import type * as TauriMenu from '@tauri-apps/api/menu'

import { apiFetch } from '@/lib/api/fetch'
import { problemOf } from '@/lib/errors/problem'
import { splitHostPath } from '@/lib/host-path'
import { wsUrl } from '@/lib/ws/url'
import { TauriWebSocket } from '@/lib/ws/tauri-transport'
import type { ContextMenuItem } from '@/components/ui/context-menu'
import { UserFacingError } from '@/lib/errors/user-facing-error'
import { IS_MOBILE } from '@/utils/platform'

// ── Terminal PTY ──────────────────────────────────────────────────────────────

// Each terminal view streams one daemon PTY session over its own socket, so it gets its own
// snapshot on attach and closing it touches no other view. On desktop the socket is the
// TauriWebSocket shim through Rust (ws_bridge.rs); both present one interface.

// Wire (api/internal/core/terminal/transport.go): daemon→client BINARY [tag][bytes] — tag 0 output
// (append), 1 snapshot (reset, apply) — or TEXT {"type":"exit","code":N}; client→daemon TEXT
// {data}, {type:"resize",cols,rows}, {type:"theme",bg,fg,dark}. Only an exit frame ends a session.
export type TerminalFrame =
  { exit?: undefined; data: Uint8Array; snapshot: boolean } | { exit: true; code: number }

// Output-frame tags: 0 = output (append), 1 = snapshot (reset, then apply).
const FRAME_SNAPSHOT = 1

// A healthy terminal socket is never silent for long — the daemon pings every
// 45s — so on desktop, where Rust owns the socket, one that delivers nothing for
// two ping periods is judged half-open and reported as a drop.
const TERMINAL_READ_IDLE_TIMEOUT_MS = 90_000

// The subset of the WebSocket interface both transports implement.
interface TerminalSocket {
  onopen: (() => void) | null
  onmessage: ((event: { data: string | ArrayBuffer }) => void) | null
  onclose: (() => void) | null
  send(data: string): void
  close(): void
}

function decodeFrame(raw: unknown): TerminalFrame | null {
  if (raw instanceof ArrayBuffer) {
    const bytes = new Uint8Array(raw)
    if (bytes.length === 0 || bytes[0] > FRAME_SNAPSHOT) return null
    return { data: bytes.subarray(1), snapshot: bytes[0] === FRAME_SNAPSHOT }
  }
  if (typeof raw !== 'string') return null
  try {
    const msg = JSON.parse(raw) as { type?: unknown; code?: unknown }
    if (msg.type !== 'exit') return null
    return { exit: true, code: typeof msg.code === 'number' ? msg.code : -1 }
  } catch {
    return null
  }
}

export type TerminalConnectionState = 'connecting' | 'open' | 'exited' | 'dropped' | 'closed'

// One view's live stream to one PTY session. Frames before the first listener (the attach
// snapshot) replay to it; input before open is queued and a theme push coalesces. A close with
// no exit frame is a drop: the view re-resolves and re-attaches, never inferring an exit.
export class TerminalConnection {
  readonly sessionId: string
  private state_: TerminalConnectionState = 'connecting'
  private readonly socket: TerminalSocket
  private outbox: string[] = []
  private pendingTheme: string | null = null
  private backlog: TerminalFrame[] = []
  private readonly listeners = new Set<(frame: TerminalFrame) => void>()
  private readonly dropListeners = new Set<() => void>()

  constructor(sessionId: string, path: string, socket?: TerminalSocket) {
    this.sessionId = sessionId
    this.socket = socket ?? openSocket(path)
    this.socket.onopen = () => {
      if (this.state_ !== 'connecting') return
      this.state_ = 'open'
      for (const frame of this.outbox) this.socket.send(frame)
      this.outbox = []
      if (this.pendingTheme) this.socket.send(this.pendingTheme)
      this.pendingTheme = null
    }
    this.socket.onmessage = (event) => {
      if (this.state_ === 'closed') return
      const frame = decodeFrame(event.data)
      if (!frame) return
      // Latched BEFORE the daemon's close arrives, so that close is not
      // mistaken for a drop.
      if (frame.exit) this.state_ = 'exited'
      if (this.listeners.size === 0) this.backlog.push(frame)
      else for (const listener of this.listeners) listener(frame)
    }
    this.socket.onclose = () => {
      if (this.state_ !== 'connecting' && this.state_ !== 'open') return
      this.state_ = 'dropped'
      for (const cb of this.dropListeners) cb()
    }
  }

  get state(): TerminalConnectionState {
    return this.state_
  }

  /** True while the transport can still carry frames (connecting or open). */
  get alive(): boolean {
    return this.state_ === 'connecting' || this.state_ === 'open'
  }

  /** Receive frames in wire order; the first listener also gets the backlog. */
  listen(onFrame: (frame: TerminalFrame) => void): () => void {
    this.listeners.add(onFrame)
    if (this.backlog.length > 0) {
      const held = this.backlog
      this.backlog = []
      for (const frame of held) onFrame(frame)
    }
    return () => this.listeners.delete(onFrame)
  }

  /** Called once if the transport dies without the session having exited. */
  onDrop(cb: () => void): () => void {
    this.dropListeners.add(cb)
    return () => this.dropListeners.delete(cb)
  }

  write(data: string): void {
    this.send(JSON.stringify({ data }))
  }

  resize(rows: number, cols: number): void {
    this.send(JSON.stringify({ type: 'resize', cols, rows }))
  }

  /** Push the host light/dark theme so a foreground app's auto theme follows a Crowbar switch
   *  (the daemon's Session.SetTheme). Only the last matters, so one sent before open coalesces. */
  setTheme(theme: { background: string; foreground: string; dark: boolean }): void {
    const frame = JSON.stringify({
      type: 'theme',
      bg: theme.background,
      fg: theme.foreground,
      dark: theme.dark,
    })
    if (this.state_ === 'open') this.socket.send(frame)
    else if (this.state_ === 'connecting') this.pendingTheme = frame
  }

  /** Detach this view: close the transport. The PTY keeps running. */
  close(): void {
    if (this.state_ === 'closed') return
    this.state_ = 'closed'
    this.listeners.clear()
    this.dropListeners.clear()
    this.socket.close()
  }

  private send(frame: string): void {
    if (this.state_ === 'open') this.socket.send(frame)
    else if (this.state_ === 'connecting') this.outbox.push(frame)
  }
}

// `qualified` may be host-qualified (`crowbar://<hostId>/v0/...`): the base a
// remote chat's terminals are built on carries its host.
function openSocket(qualified: string): TerminalSocket {
  const { hostId, path } = splitHostPath(qualified)
  if (isTauri()) {
    return new TauriWebSocket(path, { idleTimeoutMs: TERMINAL_READ_IDLE_TIMEOUT_MS, hostId })
  }
  const ws = new WebSocket(wsUrl(path, hostId))
  ws.binaryType = 'arraybuffer'
  return ws as unknown as TerminalSocket
}

// PTY routes are chat-scoped (/v0/chats/:chatId/terminals[/:id/ws]), the home's under
// /v0/projects/:p/home/terminals; callers pass the resolved base. Recorded per session for kill.
const sessionBases = new Map<string, string>()

/** Open a view's stream to an existing daemon PTY session. */
export function openTerminal(sessionId: string, base: string): TerminalConnection {
  // Recorded only once the transport exists: a constructor that throws must not
  // leave a base behind for terminalKill to act on.
  const connection = new TerminalConnection(
    sessionId,
    `${base}/${encodeURIComponent(sessionId)}/ws`,
  )
  sessionBases.set(sessionId, base)
  return connection
}

/** Create a PTY session owned by the base's chat; returns its session id. */
export async function terminalCreate(base: string, profileId?: string): Promise<string> {
  const { sessionId } = await apiFetch<{ sessionId: string }>(base, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(profileId ? { profileId } : {}),
  })
  sessionBases.set(sessionId, base)
  return sessionId
}

/** Kill a PTY session for good (tab closed); open views see its exit frame. A session whose base
 *  this page never learned can't be addressed, so nothing is sent. */
export async function terminalKill(sessionId: string): Promise<void> {
  const base = sessionBases.get(sessionId)
  sessionBases.delete(sessionId)
  if (!base) return
  try {
    await apiFetch(`${base}/${encodeURIComponent(sessionId)}`, { method: 'DELETE' })
  } catch (err) {
    // A session that already exited is what the kill wanted.
    if (problemOf(err).code !== 'not_found') throw err
  }
}

/** The daemon's live and suspended session ids for one owner only; a sibling chat on the same
 *  worktree has its own, disjoint list. */
export async function terminalListLive(base: string): Promise<string[]> {
  const list = await apiFetch<Array<{ id: string; status: string }>>(base)
  const live: string[] = []
  for (const session of list) if (session.status !== 'ended') live.push(session.id)
  return live
}

// ── File Manager ──────────────────────────────────────────────────────────────

// The `reveal_in_finder` invoke should settle well under a second. The timeout is defence in
// depth: if it ever hangs, callers (which all `.catch` to a toast) get a rejection, not silence.
const REVEAL_IN_FINDER_TIMEOUT_MS = 3_000

/** Reveal a file or directory in the OS file manager (Finder on macOS) with the
 *  item selected. `path` must be absolute. No-op outside Tauri (browser dev). */
export async function revealItemInFinder(path: string): Promise<void> {
  if (!isDesktopApp()) return
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

export async function setWindowTransparency(_enabled: boolean): Promise<void> {
  // FUTURE: invoke Tauri window transparency plugin
}

export async function setMacOSWindowAppearance(
  themeType: string,
  _transparencyEnabled: boolean,
): Promise<void> {
  // Pins the vibrancy blur view's appearance to the app theme (a LIGHT frost in light
  // mode), not NSApp.appearance, which Tauri's setTheme flips inconsistently.
  if (!isDesktopApp()) return
  await tauriInvoke('set_vibrancy_appearance', { dark: themeType === 'dark' })
}

/** Move the macOS traffic lights to `(x, y)` (logical points, window-relative); the
 *  config-time position applies only at window creation. See `useMacTrafficLightSync`. */
export async function setTrafficLightPosition(x: number, y: number): Promise<void> {
  if (!isDesktopApp()) return
  await tauriInvoke('set_traffic_light_position', { x, y })
}

/** Mirror the Toggle-console chord onto the Window-menu item so macOS cannot consume it
 *  first (Cmd+` cycles windows). `null` clears the accelerator. */
export async function setConsoleMenuChord(chord: string | null): Promise<void> {
  if (!isDesktopApp()) return
  await tauriInvoke('set_console_menu_chord', { chord })
}

/** Subscribe to the host menu item being chosen in THIS window; resolves to an unsubscribe. */
export async function onConsoleMenuToggle(handler: () => void): Promise<() => void> {
  if (!isDesktopApp()) return () => {}
  const { getCurrentWebview } = await import('@tauri-apps/api/webview')
  return getCurrentWebview().listen('console:toggle', handler)
}

// ── Native Context Menu ───────────────────────────────────────────────────────

type NativeMenuEntry = TauriMenu.MenuItem | TauriMenu.Submenu | TauriMenu.PredefinedMenuItem

// Every native resource of the open menu, in creation order: Tauri drops an item's click channel
// when its Rust wrapper drops, and an item passed inline to `Menu.new` drops once the menu is
// built — so each is created through its own constructor and kept alive here.
let openNativeMenu: Array<{ close(): Promise<void> }> = []

// Tauri keys an item's click channel by item id and removes it when ANY item
// with that id drops, so ids must be unique across menus alive at once.
let nativeMenuSeq = 0

// Loaded with the first native menu, not statically: nothing before a right-click needs it.
let menuApi: Promise<typeof TauriMenu> | null = null
const loadMenuApi = () => (menuApi ??= import('@tauri-apps/api/menu'))

async function toNativeMenuEntries(
  api: typeof TauriMenu,
  items: ContextMenuItem[],
  menuId: number,
  owned: Array<{ close(): Promise<void> }>,
  onChosen: () => void,
): Promise<NativeMenuEntry[]> {
  const entries: NativeMenuEntry[] = []
  for (const item of items) {
    let entry: NativeMenuEntry
    if (item.separator) {
      entry = await api.PredefinedMenuItem.new({ item: 'Separator' })
    } else if (item.items && item.items.length > 0) {
      entry = await api.Submenu.new({
        text: item.label,
        enabled: !item.disabled,
        items: await toNativeMenuEntries(api, item.items, menuId, owned, onChosen),
      })
    } else {
      entry = await api.MenuItem.new({
        id: `${menuId}:${item.id}`,
        text: item.label,
        enabled: !item.disabled,
        accelerator: item.shortcut,
        action: () => {
          item.onClick()
          onChosen()
        },
      })
    }
    owned.push(entry)
    entries.push(entry)
  }
  return entries
}

// Tauri delivers a click to the item's `action` through the event loop, after
// the blocking popup call returns, and closing an item drops its click channel.
// So the menu is closed once its action ran, or when the next menu replaces it.
async function closeNativeMenu(owned: Array<{ close(): Promise<void> }>): Promise<void> {
  if (openNativeMenu === owned) openNativeMenu = []
  await Promise.all(owned.map((resource) => resource.close()))
}

// Pops up the OS context menu via `popup_native_context_menu`, never `menu.popup()`: Tauri's
// plugin command holds the webview's resources lock while the menu is open, wedging every other
// resource-backed command (terminal PTYs too). `isCancelled` absorbs StrictMode's double effect.
export async function showNativeContextMenu(
  items: ContextMenuItem[],
  position: { x: number; y: number },
  isCancelled?: () => boolean,
): Promise<void> {
  await closeNativeMenu(openNativeMenu)
  const api = await loadMenuApi()
  const owned: Array<{ close(): Promise<void> }> = []
  const entries = await toNativeMenuEntries(
    api,
    items,
    nativeMenuSeq++,
    owned,
    () => void closeNativeMenu(owned),
  )
  const menu = await api.Menu.new({ items: entries })
  owned.push(menu)
  if (isCancelled?.()) {
    await closeNativeMenu(owned)
    return
  }
  openNativeMenu = owned
  try {
    await tauriInvoke('popup_native_context_menu', { rid: menu.rid, x: position.x, y: position.y })
  } catch (error) {
    await closeNativeMenu(owned)
    throw error
  }
}

// ── Tauri Helpers ─────────────────────────────────────────────────────────────

export function isTauri(): boolean {
  return typeof window !== 'undefined' && '__TAURI_INTERNALS__' in window
}

/** Tauri with a desktop window: the phone app is Tauri too, without window, menu or webview commands. */
export function isDesktopApp(): boolean {
  return isTauri() && !IS_MOBILE
}

/** A Tauri-picked filesystem path -> a URL its own webview can load
 *  (`asset://...`). Re-exported here (not imported directly) so this file
 *  stays the one place non-bridge code reaches `@tauri-apps/*` through. */
export const convertFileSrc = tauriConvertFileSrc

async function tauriInvoke<T = void>(cmd: string, args?: Record<string, unknown>): Promise<T> {
  if (!isTauri()) throw new Error(`tauriInvoke called outside Tauri: ${cmd}`)
  // Use the global injected by Tauri before any JS runs — no npm import needed
  const tauri = window as unknown as {
    __TAURI_INTERNALS__: {
      invoke: (cmd: string, args?: Record<string, unknown>) => Promise<unknown>
    }
  }
  return (await tauri.__TAURI_INTERNALS__.invoke(cmd, args)) as T
}

// Runs the desktop `diagnostics_export` command: daemon and app logs, fresh goroutine/heap dumps
// and version metadata, zipped into ~/Downloads. Resolves to the bundle's absolute path.
export async function exportDiagnostics(): Promise<string> {
  if (!isDesktopApp()) throw new UserFacingError('Diagnostics export requires the desktop app')
  return tauriInvoke<string>('diagnostics_export')
}

// ── Remote Hosts ──────────────────────────────────────────────────────────────
// Pairing and trust-on-first-contact for remote Crowbar daemon instances.

/** Tell the runtime registry where a remote host listens. No fingerprint: the pinned
 *  certificate is fixed at pairing, beside the token. Rejects a bad `hostId`
 *  (`[a-z0-9-]`, not `localhost`) or an `endpoint` that is not `host:port`. */
export async function registerRemoteHost(hostId: string, endpoint: string): Promise<void> {
  await tauriInvoke('register_remote_host', { hostId, endpoint })
}

/** Unregister a remote host and close its streams. */
export async function unregisterRemoteHost(hostId: string): Promise<void> {
  await tauriInvoke('unregister_remote_host', { hostId })
}

/** Delete a remote host's stored bearer token from the OS keychain. */
export async function forgetHostToken(hostId: string): Promise<void> {
  await tauriInvoke('forget_host_token', { hostId })
}

/** Dials `endpoint` unpinned and returns the presented certificate's fingerprint: the
 *  trust-on-first-connect step. The user MUST confirm it before `pairRemoteHost`. */
export async function probeRemoteFingerprint(endpoint: string): Promise<string> {
  return tauriInvoke<string>('probe_remote_fingerprint', { endpoint })
}

/** Redeems a pairing code against a CONFIRMED fingerprint. The token goes straight to the
 *  OS keychain; resolves to this client's id on the host (not a secret; revokes later). */
export async function pairRemoteHost(
  hostId: string,
  endpoint: string,
  fingerprint: string,
  code: string,
  name: string,
): Promise<string> {
  return tauriInvoke<string>('pair_remote_host', { hostId, endpoint, fingerprint, code, name })
}
