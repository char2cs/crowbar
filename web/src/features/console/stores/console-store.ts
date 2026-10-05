import { create } from 'zustand'
import type { ConsoleEntry, NewEntry } from '@/features/console/lib/entries'
import type { LogFrame } from '@/features/console/lib/frames'
import type { LogStreamState } from '@/features/console/lib/log-stream'
import { appendCapped } from '@/features/console/lib/ring'

/** The most lines kept; the oldest go first. Matches the daemon's own ring. */
const BUFFER_CAP = 5000

export interface ConsoleState {
  open: boolean
  stream: LogStreamState
  entries: ConsoleEntry[]
  nextId: number
  /** The highest log `seq` shown; the stream resumes after it. */
  cursor: number | null
  expandedId: number | null

  toggle: () => void
  setOpen: (open: boolean) => void
  setStream: (state: LogStreamState) => void
  /**
   * Folds frames from the log stream into the buffer. Hand it a whole replay,
   * `ready` included, in one call: a `ready` that says `reset` (the daemon
   * restarted) discards the lines already held before the replay is shown.
   */
  ingest: (frames: readonly LogFrame[]) => void
  toggleExpanded: (id: number) => void
}

function numbered(entries: readonly NewEntry[], from: number): ConsoleEntry[] {
  return entries.map((entry, i) => ({ ...entry, id: from + i }))
}

export const useConsoleStore = create<ConsoleState>((set, get) => ({
  open: false,
  stream: 'idle',
  entries: [],
  nextId: 1,
  cursor: null,
  expandedId: null,

  toggle: () => set((s) => ({ open: !s.open })),
  setOpen: (open) => set({ open }),
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
      // A replay can race the live feed; the cursor makes a repeat harmless.
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
