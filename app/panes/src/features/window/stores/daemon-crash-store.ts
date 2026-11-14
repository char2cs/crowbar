import { create } from 'zustand'

interface DaemonCrashState {
  /** The supervisor's report once it stops respawning a daemon that keeps dying; else null. */
  report: string | null
  crashed: (report: string) => void
  clear: () => void
}

// Mirrors the desktop supervisor's crash-loop state, which the health listener learns of.
export const useDaemonCrashStore = create<DaemonCrashState>()((set) => ({
  report: null,
  crashed: (report) => set({ report }),
  clear: () => set({ report: null }),
}))
