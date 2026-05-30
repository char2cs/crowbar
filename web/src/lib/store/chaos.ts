import { create } from 'zustand'

interface ChaosState {
  latency: number
  errorRate: number
  setLatency: (ms: number) => void
  setErrorRate: (rate: number) => void
  reset: () => void
}

export const useChaosStore = create<ChaosState>()((set) => ({
  latency: 0,
  errorRate: 0,
  setLatency: (latency) => set({ latency }),
  setErrorRate: (errorRate) => set({ errorRate }),
  reset: () => set({ latency: 0, errorRate: 0 }),
}))
