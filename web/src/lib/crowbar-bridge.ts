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
