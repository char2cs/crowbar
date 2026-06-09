// Crowbar system operations backed by the Go daemon's /v0 API.

import { apiFetch } from '@/lib/api'
import { wsUrl } from '@/lib/ws/url'

// ── Terminal PTY ──────────────────────────────────────────────────────────────
// Each session is a WebSocket to the daemon's PTY handler. The wire protocol is
// JSON: server→client {sessionId, data}; client→server {data} for input and
// {type:'resize', cols, rows} for SIGWINCH.

interface TerminalConnection {
  ws: WebSocket
  listener: ((data: string) => void) | null
  outputBuffer: string[]
  inputQueue: string[]
  open: boolean
}

const terminals = new Map<string, TerminalConnection>()

// Create a PTY session in the workspace and open its stream. Returns the
// sessionId, which the terminal hooks use as the connection id.
export async function terminalCreate(
  wsId: string,
  profileId?: string,
): Promise<string> {
  const { sessionId } = await apiFetch<{ sessionId: string }>(
    `/v0/workspaces/${encodeURIComponent(wsId)}/terminals`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(profileId ? { profileId } : {}),
    },
  )

  const ws = new WebSocket(wsUrl(`/v0/ws/terminals/${encodeURIComponent(sessionId)}`))
  const conn: TerminalConnection = { ws, listener: null, outputBuffer: [], inputQueue: [], open: false }

  ws.onopen = () => {
    conn.open = true
    for (const data of conn.inputQueue) ws.send(JSON.stringify({ data }))
    conn.inputQueue = []
  }
  ws.onmessage = (event) => {
    let data: string | undefined
    try {
      data = (JSON.parse(event.data as string) as { data?: string }).data
    } catch {
      return
    }
    if (typeof data !== 'string') return
    if (conn.listener) conn.listener(data)
    else conn.outputBuffer.push(data)
  }

  terminals.set(sessionId, conn)
  return sessionId
}

export async function terminalWrite(
  id: string,
  data: string,
): Promise<void> {
  const conn = terminals.get(id)
  if (!conn) return
  if (conn.open) conn.ws.send(JSON.stringify({ data }))
  else conn.inputQueue.push(data)
}

export async function terminalResize(
  id: string,
  rows: number,
  cols: number,
): Promise<void> {
  const conn = terminals.get(id)
  if (conn?.open) conn.ws.send(JSON.stringify({ type: 'resize', cols, rows }))
}

export async function terminalClose(
  id: string,
): Promise<void> {
  const conn = terminals.get(id)
  if (conn) {
    conn.ws.close()
    terminals.delete(id)
  }
  await apiFetch(`/v0/terminals/${encodeURIComponent(id)}`, { method: 'DELETE' }).catch(() => {})
}

// Register the output sink for a session, flushing any frames that arrived
// before the listener attached (e.g. the shell's first prompt).
export function terminalListen(
  id: string,
  onData: (data: string) => void,
): () => void {
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

export async function clipboardPaste(
  _targetDirectory: string,
): Promise<PastedEntry[]> {
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
  return null;
}

// ── Window Management ─────────────────────────────────────────────────────────
// FUTURE: Tauri plugin calls when Crowbar's desktop wrapper exposes them

export async function setWindowTransparency(_enabled: boolean): Promise<void> {
  // FUTURE: invoke Tauri window transparency plugin
}

export async function setMacOSWindowAppearance(
  _themeType: string,
  _transparencyEnabled: boolean,
): Promise<void> {
  // FUTURE: invoke Tauri macOS appearance plugin
}

export async function toggleMenuBar(_toggle: boolean): Promise<void> {
  // FUTURE: invoke Tauri menu bar plugin
}

// ── Browser Pane (native child webview) ──────────────────────────────────────

export function isTauri(): boolean {
  return typeof window !== 'undefined' && '__TAURI_INTERNALS__' in window
}

async function tauriInvoke(cmd: string, args?: Record<string, unknown>): Promise<void> {
  if (!isTauri()) throw new Error(`tauriInvoke called outside Tauri: ${cmd}`)
  // Use the global injected by Tauri before any JS runs — no npm import needed
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  await (window as any).__TAURI_INTERNALS__.invoke(cmd, args)
}

export async function browserPaneSync(
  bufferId: string,
  rect: { x: number; y: number; width: number; height: number },
  visible: boolean,
  // Used only on the first call (webview creation). Eliminates the race
  // between sync creating the pane and a separate navigate call on mount.
  initialUrl?: string,
): Promise<void> {
  if (!isTauri()) return
  // Tauri command expects flat args, not a nested rect object
  await tauriInvoke('browser_pane_sync', {
    bufferId,
    x: rect.x,
    y: rect.y,
    width: rect.width,
    height: rect.height,
    visible,
    initialUrl,
  })
}

export async function browserPaneNavigate(bufferId: string, url: string): Promise<void> {
  if (!isTauri()) return
  await tauriInvoke('browser_pane_navigate', { bufferId, url })
}

export async function browserPaneGoBack(bufferId: string): Promise<void> {
  if (!isTauri()) return
  await tauriInvoke('browser_pane_go_back', { bufferId })
}

export async function browserPaneGoForward(bufferId: string): Promise<void> {
  if (!isTauri()) return
  await tauriInvoke('browser_pane_go_forward', { bufferId })
}

export async function browserPaneReload(bufferId: string): Promise<void> {
  if (!isTauri()) return
  await tauriInvoke('browser_pane_reload', { bufferId })
}

export async function browserPaneClose(bufferId: string): Promise<void> {
  if (!isTauri()) return
  await tauriInvoke('browser_pane_close', { bufferId })
}
