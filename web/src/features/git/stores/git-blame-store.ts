import { create } from 'zustand'
import { failed, idle, loading, success, type Loadable } from '@/lib/loadable'
import { getBlame, type BlameEntry } from '../api/git-blame-api'

/**
 * Per-file blame from the daemon, keyed by workspace + workspace-relative
 * path (sibling worktrees share relative paths). Entries are dropped when the
 * file's buffer closes and invalidated when it is saved, so the map is
 * bounded by the open files.
 */
interface GitBlameState {
  blame: Record<string, Loadable<BlameEntry[]>>
}

export const useGitBlameStore = create<GitBlameState>(() => ({ blame: {} }))

export function blameKey(wsId: string, path: string): string {
  return `${wsId}\u0000${path}`
}

function setEntry(key: string, value: Loadable<BlameEntry[]> | undefined): void {
  useGitBlameStore.setState((s) => {
    const blame = { ...s.blame }
    if (value) blame[key] = value
    else delete blame[key]
    return { blame }
  })
}

/** Load blame once per (workspace, file) until it is invalidated. */
export async function loadBlame(wsId: string, path: string): Promise<void> {
  const key = blameKey(wsId, path)
  const current = useGitBlameStore.getState().blame[key]
  if (current && (current.status === 'success' || current.status === 'loading')) return
  setEntry(key, loading(current ?? idle()))
  try {
    const entries = await getBlame(wsId, path)
    // Invalidated (saved/closed) while in flight: drop the stale answer.
    if (useGitBlameStore.getState().blame[key]?.status !== 'loading') return
    setEntry(key, entries ? success(entries) : idle())
  } catch (error) {
    if (useGitBlameStore.getState().blame[key]?.status !== 'loading') return
    setEntry(key, failed(error as Error, current ?? idle()))
  }
}

/** Forget a file's blame (it was saved, or its buffer closed). */
export function clearBlame(wsId: string, path: string): void {
  const key = blameKey(wsId, path)
  if (useGitBlameStore.getState().blame[key]) setEntry(key, undefined)
}
