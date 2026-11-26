// Crowbar system operations backed by the Go daemon's /v0 API.

import { convertFileSrc as tauriConvertFileSrc } from '@tauri-apps/api/core'

import { apiFetch } from '@/lib/api/fetch'
import { problemOf } from '@/lib/errors/problem'
import { splitHostPath } from '@/lib/host-path'
import { wsUrl } from '@/lib/ws/url'
import { TauriWebSocket } from '@/lib/ws/tauri-transport'
import { inShell } from '@/lib/ws/in-shell'
import { ShellWebSocket } from '@/lib/ws/shell-web-socket'
import { postShellFrame } from '@/sdk/post-shell-frame'

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
  if (inShell()) {
    const shellSocket = new ShellWebSocket(path, hostId)
    shellSocket.binaryType = 'arraybuffer'
    return shellSocket
  }
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

/** Reveal a file or directory in the OS file manager with the item selected: the shell's to do.
 *  `path` must be absolute. A no-op without the shell (phone, browser). */
export function revealItemInFinder(path: string): void {
  postShellFrame({ kind: 'shell.reveal', path })
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
