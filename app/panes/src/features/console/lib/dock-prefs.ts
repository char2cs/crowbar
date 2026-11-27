import { readLocal, writeLocal } from '@/lib/local-storage'

export type ConsoleDock = 'top' | 'right' | 'bottom' | 'left'
export type ConsoleMode = 'overlay' | 'push'

export interface DockPrefs {
  dock: ConsoleDock
  mode: ConsoleMode
  size: number
}

const DEFAULT_SIZE = 420
const MIN_SIZE = 160

const DOCKS: readonly ConsoleDock[] = ['top', 'right', 'bottom', 'left']
const KEY_DOCK = 'console-dock'
const KEY_MODE = 'console-mode'
const KEY_SIZE = 'console-size'

export function nextDock(dock: ConsoleDock): ConsoleDock {
  return DOCKS[(DOCKS.indexOf(dock) + 1) % DOCKS.length]
}

export function loadDockPrefs(): DockPrefs {
  const dock = DOCKS.find((d) => d === readLocal(KEY_DOCK)) ?? 'top'
  const mode = readLocal(KEY_MODE) === 'push' ? 'push' : 'overlay'
  const stored = parseInt(readLocal(KEY_SIZE) ?? '', 10)
  const size = Number.isFinite(stored) ? Math.max(MIN_SIZE, stored) : DEFAULT_SIZE
  return { dock, mode, size }
}

export function saveDockPrefs(prefs: DockPrefs): void {
  writeLocal(KEY_DOCK, prefs.dock)
  writeLocal(KEY_MODE, prefs.mode)
  writeLocal(KEY_SIZE, String(Math.round(prefs.size)))
}
