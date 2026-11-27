import { create } from 'zustand'
import type { ConsoleEntry, NewEntry } from '@/features/console/lib/entries'
import type { LogFrame } from '@/features/console/lib/frames'
import type { LogStreamState } from '@/features/console/lib/log-stream'
import {
  loadDockPrefs,
  nextDock,
  saveDockPrefs,
  type ConsoleDock,
  type ConsoleMode,
} from '@/features/console/lib/dock-prefs'
import { appendCapped } from '@/features/console/lib/ring'

const BUFFER_CAP = 5000

export interface ConsoleState {
  open: boolean
  dock: ConsoleDock
  mode: ConsoleMode
  size: number
  stream: LogStreamState
  entries: ConsoleEntry[]
  nextId: number
  cursor: number | null
  expandedId: number | null

  toggle: () => void
  setOpen: (open: boolean) => void
  cycleDock: () => void
  toggleMode: () => void
  setSize: (size: number) => void
  setStream: (state: LogStreamState) => void
  ingest: (frames: readonly LogFrame[]) => void
  toggleExpanded: (id: number) => void
}

function numbered(entries: readonly NewEntry[], from: number): ConsoleEntry[] {
  return entries.map((entry, i) => ({ ...entry, id: from + i }))
}

function persist({ dock, mode, size }: ConsoleState): void {
  saveDockPrefs({ dock, mode, size })
}

export const useConsoleStore = create<ConsoleState>((set, get) => ({
  open: false,
  ...loadDockPrefs(),
  stream: 'idle',
  entries: [],
  nextId: 1,
  cursor: null,
  expandedId: null,

  toggle: () => set((s) => ({ open: !s.open })),
  setOpen: (open) => set({ open }),
  cycleDock: () => {
    set((s) => ({ dock: nextDock(s.dock) }))
    persist(get())
  },
  toggleMode: () => {
    set((s) => ({ mode: s.mode === 'overlay' ? 'push' : 'overlay' }))
    persist(get())
  },
  setSize: (size) => {
    set({ size })
    persist(get())
  },
  setStream: (stream) => set({ stream }),

  ingest: (frames) => {
    const reset = frames.some((f) => f.type === 'ready' && f.reset)
    let cursor = reset ? null : get().cursor
    const added: NewEntry[] = []
    if (reset) added.push({ kind: 'restarted' })

    for (const frame of frames) {
      if (frame.type === 'raw') {
        added.push({ kind: 'raw', text: frame.text })
        continue
      }
      if (frame.type !== 'log') continue
      if (cursor !== null && frame.record.seq <= cursor) continue
      cursor = frame.record.seq
      added.push({ kind: 'log', record: frame.record })
    }

    set((s) => ({
      cursor,
      expandedId: reset ? null : s.expandedId,
      nextId: s.nextId + added.length,
      entries: appendCapped(reset ? [] : s.entries, numbered(added, s.nextId), BUFFER_CAP),
    }))
  },

  toggleExpanded: (id) => set((s) => ({ expandedId: s.expandedId === id ? null : id })),
}))
