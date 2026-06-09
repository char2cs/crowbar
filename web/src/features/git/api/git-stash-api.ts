import { apiFetch } from '@/lib/api'
import type { GitStash } from '../types/git-types'

interface StashDTO {
  id: string
  message: string
  date: string
  filesChanged: number
}

function base(
  wsId: string,
): string {
  return `/v0/workspaces/${encodeURIComponent(wsId)}/git`
}

// Resolve a positional stash index to the backend's stash id, which the
// restore/drop routes key on.
async function stashIdAt(
  wsId: string,
  index: number,
): Promise<string | null> {
  try {
    const stashes = await apiFetch<StashDTO[]>(`${base(wsId)}/stashes`)
    return stashes[index]?.id ?? null
  } catch {
    return null
  }
}

export const getStashes = async (wsId: string): Promise<GitStash[]> => {
  try {
    const stashes = await apiFetch<StashDTO[]>(`${base(wsId)}/stashes`)
    return stashes.map((s, index) => ({ index, message: s.message, date: s.date }))
  } catch {
    return []
  }
}

export const createStash = async (wsId: string, message?: string): Promise<boolean> => {
  try {
    await apiFetch(`${base(wsId)}/stash`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(message ? { message } : {}),
    })
    return true
  } catch (error) {
    console.error('git stash failed:', error)
    return false
  }
}

export const applyStash = async (wsId: string, stashIndex: number): Promise<boolean> => {
  const id = await stashIdAt(wsId, stashIndex)
  if (!id) return false
  try {
    await apiFetch(`${base(wsId)}/stash/${encodeURIComponent(id)}`, { method: 'POST' })
    return true
  } catch (error) {
    console.error('git stash apply failed:', error)
    return false
  }
}

export const dropStash = async (wsId: string, stashIndex: number): Promise<boolean> => {
  const id = await stashIdAt(wsId, stashIndex)
  if (!id) return false
  try {
    await apiFetch(`${base(wsId)}/stash/${encodeURIComponent(id)}`, { method: 'DELETE' })
    return true
  } catch (error) {
    console.error('git stash drop failed:', error)
    return false
  }
}

export const popStash = async (wsId: string, stashIndex = 0): Promise<boolean> => {
  const applied = await applyStash(wsId, stashIndex)
  if (!applied) return false
  return dropStash(wsId, stashIndex)
}
