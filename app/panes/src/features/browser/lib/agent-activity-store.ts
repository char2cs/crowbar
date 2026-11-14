import { create } from 'zustand'

interface BrowserAgentState {
  /** Tab id → agent commands in flight on it. */
  active: Record<string, number>
  begin: (tabId: string) => void
  end: (tabId: string) => void
}

export const useBrowserAgentStore = create<BrowserAgentState>((set) => ({
  active: {},
  begin: (tabId) => set((s) => ({ active: { ...s.active, [tabId]: (s.active[tabId] ?? 0) + 1 } })),
  end: (tabId) =>
    set((s) => {
      const left = (s.active[tabId] ?? 1) - 1
      const { [tabId]: _gone, ...rest } = s.active
      return { active: left > 0 ? { ...rest, [tabId]: left } : rest }
    }),
}))
