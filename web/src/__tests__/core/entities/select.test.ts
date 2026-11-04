import { beforeEach, describe, expect, it, vi } from 'vitest'
import { act, renderHook } from '@testing-library/react'
import { entityStore } from '@/core/entities/entity-store'
import {
  readSelect,
  selectHomeTrees,
  selectProjectsSynced,
  selectRepo,
  selectRepos,
  selectReviewThreads,
  selectWorkspace,
  useSelect,
} from '@/core/entities/select'
import type { Chat, Repo } from '@/core/entities/repo-types'
import type { ThreadDTO } from '@/lib/types'
import {
  projectDTO,
  resetEntities,
  seedHomeTree,
  seedProjects,
  seedRepos,
} from '@/__tests__/__fixtures__/sidebar-entities'

const thread = (id: string, workspaceId: string) => ({ id, workspaceId }) as ThreadDTO

beforeEach(() => {
  resetEntities()
  entityStore.setState((st) => ({
    byKind: {
      ...st.byKind,
      review_thread: {
        a: { v: 1, data: thread('a', 'w1') },
        b: { v: 1, data: thread('b', 'w2') },
        c: { v: 2, data: thread('c', 'w1') },
      },
    },
  }))
})

describe('selectReviewThreads', () => {
  it('selectReviewThreads returns the workspace threads and keeps identity when nothing changed', () => {
    const first = selectReviewThreads(entityStore.getState(), 'w1')
    expect(first.map((t) => t.id)).toEqual(['a', 'c'])

    entityStore.setState({ epochs: { local: 'other' } })
    expect(selectReviewThreads(entityStore.getState(), 'w1')).toBe(first)

    const { byKind } = entityStore.getState()
    entityStore.setState({
      byKind: {
        ...byKind,
        review_thread: { ...byKind.review_thread, d: { v: 3, data: thread('d', 'w1') } },
      },
    })
    expect(selectReviewThreads(entityStore.getState(), 'w1').map((t) => t.id)).toEqual([
      'a',
      'c',
      'd',
    ])
  })
})

const chat = (id: string, repoId: string): Chat => ({ id, repoId, title: id, order: 0 })

const REPO: Repo = {
  id: 'r1',
  projectId: 'p1',
  name: 'crowbar',
  avatarLabel: 'C',
  avatarColor: 'bg-indigo-700',
  defaultWorkspaceId: 'ws-default',
  workspaces: [
    { id: 'ws-a', branch: 'feat/a', age: '' },
    { id: 'ws-b', branch: 'feat/b', age: '' },
  ],
  chats: [chat('c-repo', 'r1')],
}
const OTHER: Repo = { ...REPO, id: 'r2', projectId: 'p2', workspaces: [], chats: undefined }
const HOME_CHAT = chat('c-home', '')

describe('sidebar selectors over the entity store', () => {
  beforeEach(() => {
    seedRepos([REPO, OTHER])
    seedHomeTree('p1', { chats: [HOME_CHAT], folders: [] })
  })

  it('selectRepos answers the same array while nothing changes', () => {
    expect(readSelect(selectRepos)).toBe(readSelect(selectRepos))
  })

  it('selectRepo finds a repo by id and nothing for an unknown one', () => {
    expect(readSelect((s) => selectRepo(s, 'r2'))).toMatchObject({ id: 'r2', projectId: 'p2' })
    expect(readSelect((s) => selectRepo(s, 'gone'))).toBeUndefined()
  })

  it('selectWorkspace finds a tree workspace in any repo but never a default checkout', () => {
    expect(readSelect((s) => selectWorkspace(s, 'ws-b'))).toMatchObject({ branch: 'feat/b' })
    expect(readSelect((s) => selectWorkspace(s, 'ws-default'))).toBeUndefined()
  })

  it('selectHomeTrees gives every project a home tree, even an empty one', () => {
    seedProjects([projectDTO({ id: 'p1' }), projectDTO({ id: 'p3' })])
    const trees = readSelect(selectHomeTrees)
    expect(trees.p1?.chats.map((c) => c.id)).toEqual(['c-home'])
    expect(trees.p3).toEqual({ chats: [], folders: [] })
  })

  it('selectProjectsSynced holds once the projects scope lands, and only then', () => {
    expect(readSelect(selectProjectsSynced)).toBe(true)
    resetEntities()
    expect(readSelect(selectProjectsSynced)).toBe(false)
  })

  it('useSelect re-renders only when its answer changes identity', () => {
    let renders = 0
    const { result } = renderHook(() => {
      renders++
      return useSelect((s) => selectWorkspace(s, 'ws-a'))
    })
    const first = result.current
    expect(first).toMatchObject({ id: 'ws-a' })
    const before = renders

    act(() => seedHomeTree('p2', { chats: [], folders: [] }))
    act(() => seedRepos([REPO, { ...OTHER, name: 'renamed' }]))
    expect(result.current).toBe(first)
    expect(renders).toBe(before)

    act(() => seedRepos([{ ...REPO, workspaces: [{ ...REPO.workspaces[0], added: 3 }] }]))
    expect(result.current).toMatchObject({ id: 'ws-a', added: 3 })
    expect(renders).toBe(before + 1)
  })

  it('useSelect lets go of the store on unmount', () => {
    const released = vi.fn()
    const subscribe = entityStore.subscribe
    vi.spyOn(entityStore, 'subscribe').mockImplementation((listener) => {
      const stop = subscribe(listener)
      return () => (released(), stop())
    })
    const { unmount } = renderHook(() => useSelect(selectRepos))
    unmount()
    expect(released).toHaveBeenCalledTimes(1)
    vi.restoreAllMocks()
  })
})
