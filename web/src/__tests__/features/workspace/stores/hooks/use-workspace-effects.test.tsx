import type { ReactNode } from 'react'
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, waitFor } from '@testing-library/react'
import { useWorkspaceEffects } from '@/features/workspace/stores/hooks/use-workspace-effects'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import {
  createWorkspaceStore,
  type WorkspaceStore,
} from '@/features/workspace/stores/workspace-store'
import { recordWorkspaceScope, setWorkspaceScope } from '@/lib/workspace-scope'

const { fetchFileTree, subscribe, unsubscribes, fetchGitData } = vi.hoisted(() => {
  const unsubscribes: Array<ReturnType<typeof vi.fn>> = []
  return {
    fetchFileTree: vi.fn(),
    unsubscribes,
    subscribe: vi.fn(() => {
      const unsubscribe = vi.fn()
      unsubscribes.push(unsubscribe)
      return unsubscribe
    }),
    fetchGitData: vi.fn(),
  }
})

vi.mock('@/features/files/lib/file-tree-api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/features/files/lib/file-tree-api')>()
  return { ...actual, fetchFileTree }
})
vi.mock('@/features/git/api/git-data-api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/features/git/api/git-data-api')>()
  return { ...actual, fetchGitData }
})
vi.mock('@/lib/ws/manager', () => ({ wsManager: { subscribe, send: vi.fn() } }))

const endpoints = () => (subscribe.mock.calls as unknown as [string][]).map(([ep]) => ep)

function Feeds({ store, active }: { store: WorkspaceStore; active: boolean }) {
  return (
    <WorkspaceStoreContext.Provider value={store}>
      <FeedsInner wsId={store.getState().workspaceId} active={active} />
    </WorkspaceStoreContext.Provider>
  )
}

function FeedsInner({ wsId, active }: { wsId: string; active: boolean }): ReactNode {
  useWorkspaceEffects(wsId, active)
  return null
}

const treeOf = (ws: string) => [{ name: `${ws}.ts`, path: `${ws}.ts`, isDir: false }]
const statusOf = (ws: string) => ({ branch: ws, ahead: 0, behind: 0, files: [] })

beforeEach(() => {
  vi.clearAllMocks()
  unsubscribes.length = 0
  for (const ws of ['ws-A', 'ws-B']) {
    setWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: ws, owningChatId: `chat-${ws}` })
  }
  fetchFileTree.mockImplementation(async (ws: string) => treeOf(ws))
  fetchGitData.mockImplementation(async (ws: string) => ({ status: statusOf(ws), commits: [] }))
})

describe('useWorkspaceEffects across a workspace switch', () => {
  // Clicking a tiled pane of another workspace flips which workspace is
  // active; that must select a slice, not wipe and refetch it.
  it('keeps warm data and refetches nothing when the active workspace flips A to B to A', async () => {
    const a = createWorkspaceStore('ws-A')
    const b = createWorkspaceStore('ws-B')
    const view = (active: 'ws-A' | 'ws-B') => (
      <>
        <Feeds store={a} active={active === 'ws-A'} />
        <Feeds store={b} active={active === 'ws-B'} />
      </>
    )

    const { rerender } = render(view('ws-A'))
    await waitFor(() => expect(a.getState().gitLoad).toBe('ready'))
    rerender(view('ws-B'))
    await waitFor(() => expect(b.getState().gitLoad).toBe('ready'))

    const treeA = a.getState().files
    fetchFileTree.mockClear()
    fetchGitData.mockClear()

    for (const next of ['ws-A', 'ws-B', 'ws-A'] as const) rerender(view(next))

    expect(fetchFileTree).not.toHaveBeenCalled()
    expect(fetchGitData).not.toHaveBeenCalled()
    expect(unsubscribes.length).toBeGreaterThan(0)
    expect(unsubscribes.every((u) => u.mock.calls.length === 0)).toBe(true)
    expect(a.getState().files).toBe(treeA)
    expect(a.getState().fileTreeStatus).toBe('ready')
  })

  it('never lets one workspace’s tree or git status reach another’s store', async () => {
    const a = createWorkspaceStore('ws-A')
    const b = createWorkspaceStore('ws-B')
    const view = (active: 'ws-A' | 'ws-B') => (
      <>
        <Feeds store={a} active={active === 'ws-A'} />
        <Feeds store={b} active={active === 'ws-B'} />
      </>
    )

    const { rerender } = render(view('ws-A'))
    await waitFor(() => expect(a.getState().gitLoad).toBe('ready'))

    expect(a.getState().files).toEqual(treeOf('ws-A'))
    expect(a.getState().gitStatus?.branch).toBe('ws-A')
    expect(b.getState().files).toEqual([])
    expect(b.getState().gitStatus).toBeNull()
    expect(fetchFileTree).not.toHaveBeenCalledWith('ws-B')

    rerender(view('ws-B'))
    await waitFor(() => expect(b.getState().gitLoad).toBe('ready'))
    expect(b.getState().files).toEqual(treeOf('ws-B'))
    expect(b.getState().gitStatus?.branch).toBe('ws-B')
    expect(a.getState().gitStatus?.branch).toBe('ws-A')
  })

  it('opens the files and git streams once a workspace has been shown and keeps them while hidden', async () => {
    const a = createWorkspaceStore('ws-A')
    const { rerender } = render(<Feeds store={a} active={false} />)
    expect(endpoints()).toEqual([])

    rerender(<Feeds store={a} active />)
    await waitFor(() => expect(a.getState().gitLoad).toBe('ready'))
    expect(endpoints()).toEqual(
      expect.arrayContaining(['/v0/chats/chat-ws-A/files/ws', '/v0/chats/chat-ws-A/git/status']),
    )

    subscribe.mockClear()
    rerender(<Feeds store={a} active={false} />)
    expect(subscribe).not.toHaveBeenCalled()
    expect(unsubscribes.every((u) => u.mock.calls.length === 0)).toBe(true)
  })

  it('skips the git feed for the home workspace but keeps its files stream', async () => {
    setWorkspaceScope({ projectId: 'p1', repoId: '', wsId: 'home-ws' })
    const home = createWorkspaceStore('home-ws')
    render(<Feeds store={home} active />)
    await waitFor(() => expect(home.getState().fileTreeStatus).toBe('ready'))

    expect(endpoints()).toContain('/v0/projects/p1/home/files/ws')
    expect(endpoints().some((ep) => ep.includes('/git/'))).toBe(false)
    expect(fetchGitData).not.toHaveBeenCalled()
  })

  // The route records a workspace's scope with no chat id; the sidebar adds it
  // later. Every chat-scoped route throws without it, so nothing may fire early.
  it('waits for the owning chat id before loading anything', async () => {
    setWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws-race' })
    const race = createWorkspaceStore('ws-race')
    render(<Feeds store={race} active />)

    expect(fetchFileTree).not.toHaveBeenCalled()
    expect(fetchGitData).not.toHaveBeenCalled()
    expect(endpoints()).toEqual([])

    recordWorkspaceScope({
      projectId: 'p1',
      repoId: 'r1',
      wsId: 'ws-race',
      owningChatId: 'chat-race',
    })

    await waitFor(() => expect(race.getState().gitLoad).toBe('ready'))
    expect(fetchFileTree).toHaveBeenCalledWith('ws-race')
    expect(endpoints()).toContain('/v0/chats/chat-race/git/status')
  })
})
