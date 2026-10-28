import { describe, expect, it, beforeEach } from 'vitest'
import { useFolderSignalStore } from '@/lib/store/folder-signal'
import type { Repo, Workspace } from '@/lib/store/sidebar'

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

/**
 * Offline cold start: the entity cache is the only data there is, so a repo
 * whose cached rows already carry their final ids has to draw from them.
 */
describe('markSeededFromCache', () => {
  const repo = (id: string, over: Partial<Repo> = {}): Repo => ({
    id,
    name: id,
    avatarLabel: 'R',
    avatarColor: 'bg-indigo-700',
    workspaces: [],
    ...over,
  })
  const ws = (id: string, owningChatId: string): Workspace => ({
    id,
    branch: id,
    age: '',
    order: 0,
    owningChatId,
  })

  it('marks a repo whose cached workspaces all name their owning chat', () => {
    useFolderSignalStore
      .getState()
      .markSeededFromCache([repo('r1', { workspaces: [ws('w1', 'c1')] })])
    expect(useFolderSignalStore.getState().seededRepoIds.has('r1')).toBe(true)
  })

  it('marks a repo that has cached chats only', () => {
    useFolderSignalStore
      .getState()
      .markSeededFromCache([
        repo('r1', { chats: [{ id: 'c1', repoId: 'r1', title: 't', order: 0 }] }),
      ])
    expect(useFolderSignalStore.getState().seededRepoIds.has('r1')).toBe(true)
  })

  it('leaves a repo with nothing cached unmarked', () => {
    useFolderSignalStore.getState().markSeededFromCache([repo('r1')])
    expect(useFolderSignalStore.getState().seededRepoIds.has('r1')).toBe(false)
  })

  // A row cached before the daemon minted owners would change id once the
  // network answers, which is exactly what the seeded gate exists to prevent.
  it('leaves a repo unmarked while a cached workspace has no owning chat', () => {
    useFolderSignalStore
      .getState()
      .markSeededFromCache([repo('r1', { workspaces: [ws('w1', 'c1'), ws('w2', '')] })])
    expect(useFolderSignalStore.getState().seededRepoIds.has('r1')).toBe(false)
  })

  it('hands back the SAME set when nothing new is marked', () => {
    const repos = [repo('r1', { workspaces: [ws('w1', 'c1')] })]
    useFolderSignalStore.getState().markSeededFromCache(repos)
    const before = useFolderSignalStore.getState().seededRepoIds
    useFolderSignalStore.getState().markSeededFromCache(repos)
    expect(useFolderSignalStore.getState().seededRepoIds).toBe(before)
  })
})
