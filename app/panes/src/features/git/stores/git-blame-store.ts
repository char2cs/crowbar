import { create } from 'zustand'
import { failed, idle, loading, success, type Loadable } from '@/lib/loadable'
import { getBlame, type BlameEntry } from '../api/git-blame-api'

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

export async function loadBlame(wsId: string, path: string): Promise<void> {
  const key = blameKey(wsId, path)
  const current = useGitBlameStore.getState().blame[key]
  if (current && (current.status === 'success' || current.status === 'loading')) return
  setEntry(key, loading(current ?? idle()))
  try {
    const entries = await getBlame(wsId, path)
    if (useGitBlameStore.getState().blame[key]?.status !== 'loading') return
    setEntry(key, entries ? success(entries) : idle())
  } catch (error) {
    if (useGitBlameStore.getState().blame[key]?.status !== 'loading') return
    setEntry(key, failed(error as Error, current ?? idle()))
  }
}

export function clearBlame(wsId: string, path: string): void {
  const key = blameKey(wsId, path)
  if (useGitBlameStore.getState().blame[key]) setEntry(key, undefined)
}
