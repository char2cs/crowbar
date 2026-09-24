// Crowbar system operations backed by the Go daemon's /v0 API.

import { convertFileSrc as tauriConvertFileSrc } from '@tauri-apps/api/core'
import { Menu } from '@tauri-apps/api/menu'
import type {
  MenuItemOptions,
  SubmenuOptions,
  PredefinedMenuItemOptions,
} from '@tauri-apps/api/menu'

import { apiFetch } from '@/lib/api'
import { wsUrl } from '@/lib/ws/url'
import { TauriWebSocket } from '@/lib/ws/tauri-transport'
import type { ContextMenuItem } from '@/components/ui/context-menu'

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

function openSocket(path: string): TerminalSocket {
  if (isTauri()) {
    return new TauriWebSocket(path, { idleTimeoutMs: TERMINAL_READ_IDLE_TIMEOUT_MS })
  }
  const ws = new WebSocket(wsUrl(path))
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
  sessionBases.set(sessionId, base)
  return new TerminalConnection(sessionId, `${base}/${encodeURIComponent(sessionId)}/ws`)
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
  return list.filter((s) => s.status !== 'ended').map((s) => s.id)
}

// The file clipboard used to live here as an in-memory copy/cut store whose
// `clipboardPaste` was a `return []` stub — Cmd+X then Cmd+V reported a
// completed move that never happened. It now lives in the file-explorer
// clipboard store, where paste drives the daemon's real copy/rename verbs.

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

/** Move the macOS traffic lights to `(x, y)` (logical points, window-relative)
 *  at runtime — the config-time `trafficLightPosition` in tauri.conf.json only
 *  applies once, at window creation. No-op outside Tauri (browser dev). See
 *  `useMacTrafficLightSync` for who calls this and why. */
export async function setTrafficLightPosition(x: number, y: number): Promise<void> {
  if (!isTauri()) return
  await tauriInvoke('set_traffic_light_position', { x, y })
}

// ── Native Context Menu ───────────────────────────────────────────────────────

type NativeMenuEntry = MenuItemOptions | SubmenuOptions | PredefinedMenuItemOptions

function toNativeMenuEntries(items: ContextMenuItem[]): NativeMenuEntry[] {
  return items.map((item): NativeMenuEntry => {
    if (item.separator) {
      return { item: 'Separator' }
    }
    if (item.items && item.items.length > 0) {
      return {
        text: item.label,
        enabled: !item.disabled,
        items: toNativeMenuEntries(item.items),
      }
    }
    return {
      id: item.id,
      text: item.label,
      enabled: !item.disabled,
      accelerator: item.shortcut,
      action: () => item.onClick(),
    }
  })
}

/** Pops up the OS's own context menu. Always closes the underlying native
 * resource handle when the popup dismisses, whether an item was picked or
 * the popup was closed with no selection.
 *
 * Deliberately does NOT call `menu.popup()` (the JS method `@tauri-apps/api/menu`
 * provides) — that invokes Tauri's built-in `plugin:menu|popup` command, which has
 * a confirmed deadlock: it holds the webview's global resources-table lock for the
 * entire, open-ended time the menu stays open, wedging every other resource-backed
 * Tauri command (including this app's own terminal PTY channels) until it's
 * dismissed. `popup_native_context_menu` (`desktop/src-tauri/src/lib.rs`) is a
 * from-scratch command that does the same thing without holding that lock across
 * the blocking call. `menu.close()` below is unaffected — the generic
 * `plugin:resources|close` command it calls was never the buggy one.
 *
 * `isCancelled` guards against React StrictMode's dev-only double-invoke of
 * effects (setup → cleanup → setup again, synchronously, before this
 * function's first `await` can resolve): the caller flips its own flag in the
 * FIRST invocation's cleanup, then this function checks it right after
 * `Menu.new()` resolves. Without this, both invocations would go on to call
 * `Menu.new()` and pop up a REAL native menu each — two live, stacked
 * NSMenu tracking sessions — because a native popup, unlike a mocked one, has
 * already been dispatched to Rust by the time an effect's own `cancelled`
 * flag would normally stop it. Dismissing the top one then looks like "the
 * menu reopens": the second one is still sitting there underneath. */
export async function showNativeContextMenu(
  items: ContextMenuItem[],
  position: { x: number; y: number },
  isCancelled?: () => boolean,
): Promise<void> {
  const menu = await Menu.new({ items: toNativeMenuEntries(items) })
  if (isCancelled?.()) {
    await menu.close()
    return
  }
  try {
    await tauriInvoke('popup_native_context_menu', { rid: menu.rid, x: position.x, y: position.y })
  } finally {
    await menu.close()
  }
}

// ── Tauri Helpers ─────────────────────────────────────────────────────────────

export function isTauri(): boolean {
  return typeof window !== 'undefined' && '__TAURI_INTERNALS__' in window
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
  if (!isTauri()) throw new Error('Diagnostics export requires the desktop app')
  return tauriInvoke<string>('diagnostics_export')
}
