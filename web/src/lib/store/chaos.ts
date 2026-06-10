import { create } from 'zustand'
import { persist } from 'zustand/middleware'
import { getDB } from '@/lib/persistence/idb'

export type Scenario = 'extreme' | 'normal' | 'empty'

export type FaultKey =
  | 'workspaces'
  | 'projects'
  | 'file-tree'
  | 'file-content'
  | 'branch-diff'
  | 'branch-threads'
  | 'branch-description'
  | 'branch-chats'
  | 'git-commits'
  | 'git-status'
  | 'git-branches'
  | 'markdown-chat'
  | 'chats'

export const FAULT_KEYS: FaultKey[] = [
  'workspaces',
  'projects',
  'file-tree',
  'file-content',
  'branch-diff',
  'branch-threads',
  'branch-description',
  'branch-chats',
  'git-commits',
  'git-status',
  'git-branches',
  'markdown-chat',
  'chats',
]

export const FAULT_LABELS: Record<FaultKey, string> = {
  workspaces: 'Workspaces',
  projects: 'Projects',
  'file-tree': 'File tree',
  'file-content': 'File content',
  'branch-diff': 'Branch diff',
  'branch-threads': 'Review threads',
  'branch-description': 'PR description',
  'branch-chats': 'Branch chats',
  'git-commits': 'Git commits',
  'git-status': 'Git status',
  'git-branches': 'Git branches',
  'markdown-chat': 'Chat history',
  chats: 'Chats',
}

const DEFAULT_FAULTS: Record<FaultKey, number> = {
  workspaces: 0,
  projects: 0,
  'file-tree': 0,
  'file-content': 0,
  'branch-diff': 0,
  'branch-threads': 0,
  'branch-description': 0,
  'branch-chats': 0,
  'git-commits': 0,
  'git-status': 0,
  'git-branches': 0,
  'markdown-chat': 0,
  chats: 0,
}

interface ChaosState {
  latency: number
  errorRate: number
  scenario: Scenario
  faults: Record<FaultKey, number>
  setLatency: (ms: number) => void
  setErrorRate: (rate: number) => void
  setScenario: (s: Scenario) => void
  setFault: (key: FaultKey, pct: number) => void
  reset: () => void
  resetFaults: () => void
  applyScenario: (s: Scenario) => Promise<void>
}

export const useChaosStore = create<ChaosState>()(
  persist(
    (set) => ({
      latency: 0,
      errorRate: 0,
      scenario: 'normal' as Scenario,
      faults: { ...DEFAULT_FAULTS },
      setLatency: (latency) => set({ latency }),
      setErrorRate: (errorRate) => set({ errorRate }),
      setScenario: (scenario) => set({ scenario }),
      setFault: (key, pct) => set((s) => ({ faults: { ...s.faults, [key]: pct } })),
      reset: () => set({ latency: 0, errorRate: 0 }),
      resetFaults: () => set({ faults: { ...DEFAULT_FAULTS } }),
      applyScenario: async (newScenario) => {
        set({ scenario: newScenario })
        try {
          const db = await getDB()
          await Promise.all([
            db.clear('branch-review'),
            db.clear('workspace-hierarchy'),
            db.clear('sidebar-ui'),
            db.clear('workspaces-data'),
            db.clear('git-data'),
            db.clear('file-tree-data'),
            db.clear('branch-review-data'),
            db.clear('chat-history'),
            db.clear('projects-data'),
          ])
        } catch {
          /* best effort */
        }
        window.location.reload()
      },
    }),
    {
      name: 'crowbar.chaos',
      partialize: (s) => ({ scenario: s.scenario, faults: s.faults }),
      // Persisted state may be partial or stale (older schema, manually edited,
      // or a fault key added since it was written). Deep-merge `faults` over the
      // full defaults so every key is always present — a missing key would render
      // an empty "%" and feed `undefined` into the slider.
      merge: (persisted, current) => {
        const p = (persisted ?? {}) as Partial<ChaosState>
        return {
          ...current,
          ...p,
          faults: { ...DEFAULT_FAULTS, ...(p.faults ?? {}) },
        }
      },
    },
  ),
)
