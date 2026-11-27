import { convertFileSrc as tauriConvertFileSrc } from '@tauri-apps/api/core'
import type * as TauriMenu from '@tauri-apps/api/menu'

import { apiFetch } from '@/lib/api/fetch'
import { problemOf } from '@/lib/errors/problem'
import { splitHostPath } from '@/lib/host-path'
import { wsUrl } from '@/lib/ws/url'
import { TauriWebSocket } from '@/lib/ws/tauri-transport'
import { inShell } from '@/lib/ws/in-shell'
import { ShellWebSocket } from '@/lib/ws/shell-web-socket'
import { postShellFrame } from '@/sdk/post-shell-frame'
import type { ContextMenuItem } from '@/components/ui/context-menu'
import { UserFacingError } from '@/lib/errors/user-facing-error'
import { IS_MOBILE } from '@/utils/platform'

export type TerminalFrame =
  { exit?: undefined; data: Uint8Array; snapshot: boolean } | { exit: true; code: number }

const FRAME_SNAPSHOT = 1

const TERMINAL_READ_IDLE_TIMEOUT_MS = 90_000

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

  get alive(): boolean {
    return this.state_ === 'connecting' || this.state_ === 'open'
  }

  listen(onFrame: (frame: TerminalFrame) => void): () => void {
    this.listeners.add(onFrame)
    if (this.backlog.length > 0) {
      const held = this.backlog
      this.backlog = []
      for (const frame of held) onFrame(frame)
    }
    return () => this.listeners.delete(onFrame)
  }

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

const sessionBases = new Map<string, string>()

export function openTerminal(sessionId: string, base: string): TerminalConnection {
  const connection = new TerminalConnection(
    sessionId,
    `${base}/${encodeURIComponent(sessionId)}/ws`,
  )
  sessionBases.set(sessionId, base)
  return connection
}

export async function terminalCreate(base: string, profileId?: string): Promise<string> {
  const { sessionId } = await apiFetch<{ sessionId: string }>(base, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(profileId ? { profileId } : {}),
  })
  sessionBases.set(sessionId, base)
  return sessionId
}

export async function terminalKill(sessionId: string): Promise<void> {
  const base = sessionBases.get(sessionId)
  sessionBases.delete(sessionId)
  if (!base) return
  try {
    await apiFetch(`${base}/${encodeURIComponent(sessionId)}`, { method: 'DELETE' })
  } catch (err) {
    if (problemOf(err).code !== 'not_found') throw err
  }
}

export async function terminalListLive(base: string): Promise<string[]> {
  const list = await apiFetch<Array<{ id: string; status: string }>>(base)
  const live: string[] = []
  for (const session of list) if (session.status !== 'ended') live.push(session.id)
  return live
}

const REVEAL_IN_FINDER_TIMEOUT_MS = 3_000

export async function revealItemInFinder(path: string): Promise<void> {
  if (postShellFrame({ kind: 'shell.reveal', path })) return
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

export async function setMacOSWindowAppearance(themeType: string): Promise<void> {
  if (!isDesktopApp()) return
  await tauriInvoke('set_vibrancy_appearance', { dark: themeType === 'dark' })
}

type NativeMenuEntry = TauriMenu.MenuItem | TauriMenu.Submenu | TauriMenu.PredefinedMenuItem

let openNativeMenu: Array<{ close(): Promise<void> }> = []

let nativeMenuSeq = 0

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

async function closeNativeMenu(owned: Array<{ close(): Promise<void> }>): Promise<void> {
  if (openNativeMenu === owned) openNativeMenu = []
  await Promise.all(owned.map((resource) => resource.close()))
}

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

export function isTauri(): boolean {
  return typeof window !== 'undefined' && '__TAURI_INTERNALS__' in window
}

export function isDesktopApp(): boolean {
  return isTauri() && !IS_MOBILE
}

export const convertFileSrc = tauriConvertFileSrc

async function tauriInvoke<T = void>(cmd: string, args?: Record<string, unknown>): Promise<T> {
  if (!isTauri()) throw new Error(`tauriInvoke called outside Tauri: ${cmd}`)
  const tauri = window as unknown as {
    __TAURI_INTERNALS__: {
      invoke: (cmd: string, args?: Record<string, unknown>) => Promise<unknown>
    }
  }
  return (await tauri.__TAURI_INTERNALS__.invoke(cmd, args)) as T
}

export async function exportDiagnostics(): Promise<string> {
  if (!isDesktopApp()) throw new UserFacingError('Diagnostics export requires the desktop app')
  return tauriInvoke<string>('diagnostics_export')
}

export async function registerRemoteHost(hostId: string, endpoint: string): Promise<void> {
  await tauriInvoke('register_remote_host', { hostId, endpoint })
}

export async function unregisterRemoteHost(hostId: string): Promise<void> {
  await tauriInvoke('unregister_remote_host', { hostId })
}

export async function forgetHostToken(hostId: string): Promise<void> {
  await tauriInvoke('forget_host_token', { hostId })
}

export async function probeRemoteFingerprint(endpoint: string): Promise<string> {
  return tauriInvoke<string>('probe_remote_fingerprint', { endpoint })
}

export async function pairRemoteHost(
  hostId: string,
  endpoint: string,
  fingerprint: string,
  code: string,
  name: string,
): Promise<string> {
  return tauriInvoke<string>('pair_remote_host', { hostId, endpoint, fingerprint, code, name })
}
