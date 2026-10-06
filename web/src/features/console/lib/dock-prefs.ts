export type ConsoleDock = 'top' | 'right' | 'bottom' | 'left'
export type ConsoleMode = 'overlay' | 'push'

export interface DockPrefs {
  dock: ConsoleDock
  mode: ConsoleMode
  /** Pixels along the axis facing the app: height for top/bottom, width for left/right. */
  size: number
}

export const DEFAULT_SIZE = 420
const MIN_SIZE = 160
/** The most of the window the console may take. */
const MAX_FRACTION = 0.8

const DOCKS: readonly ConsoleDock[] = ['top', 'right', 'bottom', 'left']
const KEY_DOCK = 'console-dock'
const KEY_MODE = 'console-mode'
const KEY_SIZE = 'console-size'

export function clampSize(size: number, windowExtent: number): number {
  const max = Math.max(MIN_SIZE, Math.floor(windowExtent * MAX_FRACTION))
  return Math.min(max, Math.max(MIN_SIZE, Math.round(size)))
}

export function nextDock(dock: ConsoleDock): ConsoleDock {
  return DOCKS[(DOCKS.indexOf(dock) + 1) % DOCKS.length]
}

function read(key: string): string | null {
  try {
    return localStorage.getItem(key)
  } catch {
    return null
  }
}

export function loadDockPrefs(): DockPrefs {
  const dock = DOCKS.find((d) => d === read(KEY_DOCK)) ?? 'top'
  const mode = read(KEY_MODE) === 'push' ? 'push' : 'overlay'
  const stored = parseInt(read(KEY_SIZE) ?? '', 10)
  const size = Number.isFinite(stored) ? Math.max(MIN_SIZE, stored) : DEFAULT_SIZE
  return { dock, mode, size }
}

export function saveDockPrefs(prefs: DockPrefs): void {
  try {
    localStorage.setItem(KEY_DOCK, prefs.dock)
    localStorage.setItem(KEY_MODE, prefs.mode)
    localStorage.setItem(KEY_SIZE, String(Math.round(prefs.size)))
  } catch {
    // storage unavailable
  }
}
