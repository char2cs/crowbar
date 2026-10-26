// Crowbar system operations backed by the Go daemon's /v0 API.

import { convertFileSrc as tauriConvertFileSrc } from '@tauri-apps/api/core'
import type * as TauriMenu from '@tauri-apps/api/menu'

import { apiFetch } from '@/lib/api'
import { splitHostPath } from '@/lib/host-path'
import { wsUrl } from '@/lib/ws/url'
import { TauriWebSocket } from '@/lib/ws/tauri-transport'
import type { ContextMenuItem } from '@/components/ui/context-menu'
import { IS_MOBILE } from '@/utils/platform'

// ── Terminal PTY ──────────────────────────────────────────────────────────────
// A terminal VIEW streams one daemon PTY session over its own WebSocket — a
// TerminalConnection. Every view gets its own transport: the daemon sends each
// attached client its own snapshot, so a view is always painted on attach, and
// closing one view's transport never touches another's.
//
// Wire protocol (see api/internal/core/terminal/transport.go):
//   daemon → client  BINARY  [tag][bytes]   tag 0 = output (append),
//                                           tag 1 = snapshot (reset, then apply)
//                    TEXT    {"type":"exit","code":N}  — the process exited
//   client → daemon  TEXT    {data} input, {type:"resize",cols,rows},
//                             {type:"theme",bg,fg,dark}
//
// A socket that closes WITHOUT an exit frame is a transport drop: the view
// re-resolves and re-attaches. An exit frame is the only thing that ends a
// terminal — nothing here ever infers one from what the user typed.
//
// On the desktop app the browser WebSocket cannot reach the daemon (its only
// endpoint is the `crowbar://` unix-socket proxy); there the TauriWebSocket
// shim dials it through Rust (desktop/src-tauri/src/ws_bridge.rs). Both
// present the same socket interface, so this module has one code path.

// One parsed daemon→client terminal frame, in wire order.
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

/**
 * One view's live stream to one daemon PTY session.
 *
 * Frames that arrive before the first listener (the attach snapshot, usually)
 * are held and replayed to it. Input sent before the socket opens is queued and
 * flushed in order on open; a theme push coalesces to the last one.
 */
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

  /**
   * Push the host light/dark theme so a foreground app's automatic theme can
   * follow a Crowbar theme switch (see the daemon's Session.SetTheme). Only the
   * LAST theme matters, so one sent before the socket opens coalesces.
   */
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

// PTY routes are CHAT-scoped (/v0/chats/:chatId/terminals[/:id/ws]); the home
// workspace's are under /v0/projects/:projectId/home/terminals. Callers pass the
// resolved base (workspace-scope-url) so the route shape lives in one place.
// The base is recorded per session so a later kill can build its DELETE.
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

/**
 * Kill a PTY session for good (the tab was closed). Open views see its exit
 * frame. A session whose base this page never learned (created before a
 * reload and never re-opened) cannot be addressed; nothing is sent.
 */
export async function terminalKill(sessionId: string): Promise<void> {
  const base = sessionBases.get(sessionId)
  sessionBases.delete(sessionId)
  if (!base) return
  await apiFetch(`${base}/${encodeURIComponent(sessionId)}`, { method: 'DELETE' }).catch(() => {})
}

/**
 * The daemon's live (and suspended) session ids for one owner — ONLY that
 * owner's; a sibling chat on the same worktree has its own, disjoint list.
 */
export async function terminalListLive(base: string): Promise<string[]> {
  const list = await apiFetch<Array<{ id: string; status: string }>>(base)
  const live: string[] = []
  for (const session of list) if (session.status !== 'ended') live.push(session.id)
  return live
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

// Every native resource of the open menu, in creation order. Tauri drops an
// item's click channel when its Rust wrapper drops, and an item passed inline
// to `Menu.new` is dropped as soon as the menu is built — so each item is
// created through its own constructor and stays alive here.
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

/** Pops up the OS's own context menu.
 *
 * Deliberately does NOT call `menu.popup()` (the JS method `@tauri-apps/api/menu`
 * provides) — that invokes Tauri's built-in `plugin:menu|popup` command, which has
 * a confirmed deadlock: it holds the webview's global resources-table lock for the
 * entire, open-ended time the menu stays open, wedging every other resource-backed
 * Tauri command (including this app's own terminal PTY channels) until it's
 * dismissed. `popup_native_context_menu` (`desktop/src-tauri/src/lib.rs`) is a
 * from-scratch command that does the same thing without holding that lock across
 * the blocking call.
 *
 * `isCancelled` guards against React StrictMode's dev-only double-invoke of
 * effects, which would otherwise pop two stacked native menus from one right-click. */
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

/**
 * Runs the desktop `diagnostics_export` command: bundles the daemon log
 * (panic traces, watchdog goroutine dumps), the app log, fresh
 * goroutine/heap dumps from the live daemon, and version metadata into a
 * zip in ~/Downloads. Resolves to the bundle's absolute path.
 */
export async function exportDiagnostics(): Promise<string> {
  if (!isDesktopApp()) throw new Error('Diagnostics export requires the desktop app')
  return tauriInvoke<string>('diagnostics_export')
}

// ── Remote Hosts ──────────────────────────────────────────────────────────────
// Pairing and trust-on-first-contact for remote Crowbar daemon instances.

/** Tell the runtime registry where a remote host listens. There is no
 *  fingerprint to pass: the certificate a host's token may be sent to is fixed
 *  at pairing, beside the token, where nothing here can change it. Rejects a
 *  `hostId` that is not lowercase letters, digits and hyphens, or is
 *  `localhost`, and an `endpoint` that is not `host:port` (`normalizeEndpoint`). */
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

/** Dials `endpoint` (`host:port`, from `normalizeEndpoint`) on first contact, unpinned, and
 *  returns the presented certificate's fingerprint. The caller MUST show this to the user and
 *  get explicit confirmation before ever calling `pairRemoteHost` with it — this is the
 *  trust-on-first-connect step, not a background lookup. */
export async function probeRemoteFingerprint(endpoint: string): Promise<string> {
  return tauriInvoke<string>('probe_remote_fingerprint', { endpoint })
}

/** Redeems a pairing code against a CONFIRMED fingerprint. The bearer token
 *  this produces is stored directly in the OS keychain by the Rust side and
 *  never returned here. Resolves to the host's id for this client — not a
 *  secret; keep it to revoke this pairing server-side later. */
export async function pairRemoteHost(
  hostId: string,
  endpoint: string,
  fingerprint: string,
  code: string,
  name: string,
): Promise<string> {
  return tauriInvoke<string>('pair_remote_host', { hostId, endpoint, fingerprint, code, name })
}
