import { describe, expect, it, beforeEach } from 'vitest'
import { useFolderSignalStore } from '@/lib/store/folder-signal'

beforeEach(() => {
  useFolderSignalStore.setState({ generations: {}, seededRepoIds: new Set() })
})

/**
 * The flag that tells "this repo has no chats" apart from "its chat seed has
 * not landed yet" — a distinction `Repo.chats` documents but nothing recorded,
 * and the one `rows-from-repo.ts` needs before it can identify a branch row by
 * the chat that owns it.
 */
describe('markTreeSeeded', () => {
  it('records the repo whose tree has been read', () => {
    useFolderSignalStore.getState().markTreeSeeded('r1')
    expect(useFolderSignalStore.getState().seededRepoIds.has('r1')).toBe(true)
  })

  it('starts out saying nothing has been read — the polarity that matters', () => {
    expect(useFolderSignalStore.getState().seededRepoIds.has('r1')).toBe(false)
  })

  it('leaves other repos alone', () => {
    useFolderSignalStore.getState().markTreeSeeded('r1')
    useFolderSignalStore.getState().markTreeSeeded('r2')
    expect([...useFolderSignalStore.getState().seededRepoIds].sort()).toEqual(['r1', 'r2'])
  })

  // Every reseed calls this, and the whole sidebar re-derives on set identity:
  // handing back a new Set each time would cost a full tree render per reseed.
  it('hands back the SAME set when the repo is already known', () => {
    useFolderSignalStore.getState().markTreeSeeded('r1')
    const before = useFolderSignalStore.getState().seededRepoIds
    useFolderSignalStore.getState().markTreeSeeded('r1')
    expect(useFolderSignalStore.getState().seededRepoIds).toBe(before)
  })
})
