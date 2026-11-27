import { create } from 'zustand'

interface GitRefreshState {
  requested: Record<string, number>
  changed: Record<string, number>
}

/** @internal */
export const useGitRefreshStore = create<GitRefreshState>(() => ({ requested: {}, changed: {} }))

export function requestGitRefresh(wsId: string): void {
  useGitRefreshStore.setState((s) => ({
    requested: { ...s.requested, [wsId]: (s.requested[wsId] ?? 0) + 1 },
  }))
}

export function markGitStatusChanged(wsId: string): void {
  useGitRefreshStore.setState((s) => ({
    changed: { ...s.changed, [wsId]: (s.changed[wsId] ?? 0) + 1 },
  }))
}

export function onGitRefreshRequested(wsId: string, listener: () => void): () => void {
  return useGitRefreshStore.subscribe((state, prev) => {
    if (state.requested[wsId] !== prev.requested[wsId]) listener()
  })
}

export function onGitStatusChanged(wsId: string, listener: () => void): () => void {
  return useGitRefreshStore.subscribe((state, prev) => {
    if (state.changed[wsId] !== prev.changed[wsId]) listener()
  })
}
