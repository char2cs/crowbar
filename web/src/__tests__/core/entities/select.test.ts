import { beforeEach, describe, expect, it, vi } from 'vitest'
import { act, renderHook } from '@testing-library/react'
import { entityStore } from '@/core/entities/entity-store'
import {
  readSelect,
  selectChat,
  selectHomeTrees,
  selectProjectsSynced,
  selectRepo,
  selectRepos,
  selectReviewThreads,
  selectWorkspace,
  useSelect,
} from '@/core/entities/select'
import type { Chat, Repo } from '@/core/entities/repo-types'
import { getInitialState, useSidebarStore } from '@/lib/store/sidebar'
import { useHomeTreeStore } from '@/lib/store/home-tree'
import { useFolderSignalStore } from '@/lib/store/folder-signal'
import type { ThreadDTO } from '@/lib/types'

const thread = (id: string, workspaceId: string) => ({ id, workspaceId }) as ThreadDTO

beforeEach(() => {
  entityStore.setState({
    byKind: {
      operation: {},
      review_thread: {
        a: { v: 1, data: thread('a', 'w1') },
        b: { v: 1, data: thread('b', 'w2') },
        c: { v: 2, data: thread('c', 'w1') },
      },
    },
  })
})

describe('selectReviewThreads', () => {
  it('selectReviewThreads returns the workspace threads and keeps identity when nothing changed', () => {
    const first = selectReviewThreads(entityStore.getState(), 'w1')
    expect(first.map((t) => t.id)).toEqual(['a', 'c'])

    entityStore.setState({ epoch: 'other' })
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

describe('sidebar selectors over the sidebar stores', () => {
  beforeEach(() => {
    useSidebarStore.setState({ ...getInitialState(), repos: [REPO, OTHER] })
    useHomeTreeStore.setState({ trees: { p1: { chats: [HOME_CHAT], folders: [] } } })
    useFolderSignalStore.setState({
      seededWorkspaceRepoIds: new Set(['r1']),
      seededRepoListProjectIds: new Set(['p1']),
    })
  })

  it('selectRepos answers the store array itself', () => {
    expect(readSelect(selectRepos)).toBe(useSidebarStore.getState().repos)
  })

  it('selectRepo finds a repo by id and nothing for an unknown one', () => {
    expect(readSelect((s) => selectRepo(s, 'r2'))).toBe(OTHER)
    expect(readSelect((s) => selectRepo(s, 'gone'))).toBeUndefined()
  })

  it('selectWorkspace finds a tree workspace in any repo but never a default checkout', () => {
    expect(readSelect((s) => selectWorkspace(s, 'ws-b'))).toBe(REPO.workspaces[1])
    expect(readSelect((s) => selectWorkspace(s, 'ws-default'))).toBeUndefined()
  })

  it('selectChat finds a repo chat, then a project-home chat', () => {
    expect(readSelect((s) => selectChat(s, 'c-repo'))).toBe(REPO.chats?.[0])
    expect(readSelect((s) => selectChat(s, 'c-home'))).toBe(HOME_CHAT)
    expect(readSelect((s) => selectChat(s, 'gone'))).toBeUndefined()
  })

  it('selectHomeTrees answers the home tree map itself', () => {
    expect(readSelect(selectHomeTrees)).toBe(useHomeTreeStore.getState().trees)
  })

  it('selectProjectsSynced asks the repo once it is known, else the project', () => {
    const synced = (projectId: string, repoId: string) =>
      readSelect((s) => selectProjectsSynced(s, projectId, repoId))
    expect(synced('p1', 'r1')).toBe(true)
    expect(synced('p2', 'r2')).toBe(false) // repo known, its workspaces unread
    expect(synced('p1', 'gone')).toBe(true) // repo unknown, its project's repos read
    expect(synced('p2', 'gone')).toBe(false)
  })

  it('useSelect re-renders only when its answer changes identity, on any store it reads', () => {
    let renders = 0
    const { result } = renderHook(() => {
      renders++
      return useSelect((s) => selectWorkspace(s, 'ws-a'))
    })
    expect(result.current).toBe(REPO.workspaces[0])
    const before = renders

    act(() => useFolderSignalStore.getState().bump('r1'))
    act(() => useHomeTreeStore.getState().setTree('p2', { chats: [], folders: [] }))
    act(() => useSidebarStore.setState({ repos: [REPO, { ...OTHER, name: 'renamed' }] }))
    expect(renders).toBe(before)

    const moved = { ...REPO.workspaces[0], added: 3 }
    act(() =>
      useSidebarStore.setState({ repos: [{ ...REPO, workspaces: [moved, REPO.workspaces[1]] }] }),
    )
    expect(result.current).toBe(moved)
    expect(renders).toBe(before + 1)
  })

  it('useSelect follows a home-tree write and a seed mark as well as a sidebar write', () => {
    const { result: home } = renderHook(() => useSelect((s) => selectChat(s, 'c-new')))
    const { result: synced } = renderHook(() =>
      useSelect((s) => selectProjectsSynced(s, 'p2', 'r2')),
    )
    expect(home.current).toBeUndefined()
    expect(synced.current).toBe(false)

    const added = chat('c-new', '')
    act(() => useHomeTreeStore.getState().setTree('p2', { chats: [added], folders: [] }))
    expect(home.current).toBe(added)
    act(() => useFolderSignalStore.getState().markWorkspacesSeeded('r2'))
    expect(synced.current).toBe(true)
  })

  it('useSelect lets go of every store it follows on unmount', () => {
    const released = vi.fn()
    const stores: { subscribe: (listener: () => void) => () => void }[] = [
      useSidebarStore,
      useHomeTreeStore,
      useFolderSignalStore,
    ]
    for (const store of stores) {
      const subscribe = store.subscribe
      vi.spyOn(store, 'subscribe').mockImplementation((listener) => {
        const stop = subscribe(listener)
        return () => (released(), stop())
      })
    }
    const { unmount } = renderHook(() => useSelect(selectRepos))
    unmount()
    expect(released).toHaveBeenCalledTimes(3)
    vi.restoreAllMocks()
  })
})
