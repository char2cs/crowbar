// Crowbar system operations — mock implementations for this session.
// FUTURE: replace each function body with the real Crowbar Go API or Tauri plugin call.

// ── Terminal PTY ──────────────────────────────────────────────────────────────
// FUTURE: WebSocket to Go PTY handler at ws://localhost/api/terminal/:id

export async function terminalWrite(
  _id: string,
  _data: string,
): Promise<void> {
  // FUTURE: ws.send(JSON.stringify({ type: 'write', id: _id, data: _data }))
}

export async function terminalResize(
  _id: string,
  _rows: number,
  _cols: number,
): Promise<void> {
  // FUTURE: ws.send(JSON.stringify({ type: 'resize', id: _id, rows: _rows, cols: _cols }))
}

export async function terminalClose(_id: string): Promise<void> {
  // FUTURE: ws.send(JSON.stringify({ type: 'close', id: _id }))
}

export function terminalListen(
  _id: string,
  _onData: (data: string) => void,
): () => void {
  // FUTURE: subscribe to WebSocket messages for terminal output
  return () => {} // no-op unlisten
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
