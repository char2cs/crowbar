import { create } from 'zustand'

/**
 * Per-workspace git refresh signals — the store-backed replacement for the
 * old global `git-status-updated` / `git-status-changed` window events, which
 * made every mounted workspace reload on any save anywhere.
 *
 *  - `requested[wsId]` bumps when something local (a save, push/pull) knows
 *    the workspace's git status is stale. The workspace's git effect reloads.
 *  - `changed[wsId]` bumps after that workspace's status actually reloaded or
 *    the daemon reported a git change; views derived from git (review outline,
 *    changed-files summary) refetch on it.
 */
interface GitRefreshState {
  requested: Record<string, number>
  changed: Record<string, number>
}

/** @internal Exported for unit tests. */
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

/** Subscribe to reload requests for one workspace. */
export function onGitRefreshRequested(wsId: string, listener: () => void): () => void {
  return useGitRefreshStore.subscribe((state, prev) => {
    if (state.requested[wsId] !== prev.requested[wsId]) listener()
  })
}

/** Subscribe to "`wsId`'s git status changed" (already coalesced upstream). */
export function onGitStatusChanged(wsId: string, listener: () => void): () => void {
  return useGitRefreshStore.subscribe((state, prev) => {
    if (state.changed[wsId] !== prev.changed[wsId]) listener()
  })
}
