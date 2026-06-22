import { describe, it, expect, vi, beforeEach } from 'vitest'
import { IDBFactory } from 'fake-indexeddb'
import { resetDB } from '@/lib/persistence/idb'
import { getAllEntities } from '@/lib/persistence/entity-cache'
import type { WorkspaceDTO } from '@/lib/types'

// The entity-stream layer is built on top of wsManager.subscribe — mock it so
// the test drives WS frames directly without standing up a real socket.
const subscribers = new Set<(data: unknown) => void>()
const unsubscribe = vi.fn()
const subscribe = vi.fn((_endpoint: string, cb: (data: unknown) => void) => {
  subscribers.add(cb)
  return () => {
    subscribers.delete(cb)
    unsubscribe()
  }
})

function emit(data: unknown): void {
  subscribers.forEach((cb) => cb(data))
}

vi.mock('@/lib/ws/manager', () => ({
  wsManager: {
    subscribe: (endpoint: string, cb: (data: unknown) => void) => subscribe(endpoint, cb),
    send: vi.fn(),
  },
}))

const { subscribeEntityStream } = await import('@/lib/ws/entity-stream')

function makeWorkspace(over: Partial<WorkspaceDTO> & { id: string }): WorkspaceDTO {
  return {
    repoId: 'r1',
    projectId: 'p1',
    branch: 'main',
    parentId: '',
    forkPointSha: '',
    status: 'new',
    working: false,
    lastError: '',
    added: 0,
    deleted: 0,
    mergeStrategy: '',
    canMergeLocally: false,
    mergeConflicts: false,
    parentBranch: '',
    prUrl: '',
    prTitle: '',
    prTargetBranch: '',
    ...over,
  }
}

beforeEach(() => {
  resetDB()
  globalThis.indexedDB = new IDBFactory()
  subscribers.clear()
  subscribe.mockClear()
  unsubscribe.mockClear()
})

describe('subscribeEntityStream', () => {
  it('seeds the entity cache by upserting all GET results', async () => {
    const seed = vi.fn(async () => [makeWorkspace({ id: 'w1' }), makeWorkspace({ id: 'w2' })])
    const onChange = vi.fn()

    subscribeEntityStream<WorkspaceDTO>({
      endpoint: '/v0/projects/p1/repos/r1/workspaces',
      store: 'crowbar_workspaces',
      seed,
      onChange,
    })

    // let the seed promise resolve
    await vi.waitFor(async () => {
      expect(await getAllEntities<WorkspaceDTO>('crowbar_workspaces')).toHaveLength(2)
    })
    expect(seed).toHaveBeenCalledTimes(1)
    expect(onChange).toHaveBeenCalled()
  })

  it('upserts a complete DTO frame by id', async () => {
    const seed = vi.fn(async () => [makeWorkspace({ id: 'w1', branch: 'main' })])
    subscribeEntityStream<WorkspaceDTO>({
      endpoint: '/v0/projects/p1/repos/r1/workspaces',
      store: 'crowbar_workspaces',
      seed,
    })
    await vi.waitFor(async () => {
      expect(await getAllEntities<WorkspaceDTO>('crowbar_workspaces')).toHaveLength(1)
    })

    emit(makeWorkspace({ id: 'w1', branch: 'feature' }))
    emit(makeWorkspace({ id: 'w2', branch: 'other' }))

    await vi.waitFor(async () => {
      const all = await getAllEntities<WorkspaceDTO>('crowbar_workspaces')
      expect(all).toHaveLength(2)
      expect(all.find((w) => w.id === 'w1')?.branch).toBe('feature')
    })
  })

  it('removes an entity when a status:"deleted" frame arrives', async () => {
    const seed = vi.fn(async () => [makeWorkspace({ id: 'w1' })])
    subscribeEntityStream<WorkspaceDTO>({
      endpoint: '/v0/projects/p1/repos/r1/workspaces',
      store: 'crowbar_workspaces',
      seed,
    })
    await vi.waitFor(async () => {
      expect(await getAllEntities<WorkspaceDTO>('crowbar_workspaces')).toHaveLength(1)
    })

    emit(makeWorkspace({ id: 'w1', status: 'deleted' }))

    await vi.waitFor(async () => {
      expect(await getAllEntities<WorkspaceDTO>('crowbar_workspaces')).toHaveLength(0)
    })
  })

  it('re-seeds (full GET) on the reconnect sentinel, not a corrupt upsert', async () => {
    let seedCalls = 0
    const seed = vi.fn(async () => {
      seedCalls++
      return seedCalls === 1
        ? [makeWorkspace({ id: 'w1' })]
        : [makeWorkspace({ id: 'w1' }), makeWorkspace({ id: 'w2' })]
    })
    subscribeEntityStream<WorkspaceDTO>({
      endpoint: '/v0/projects/p1/repos/r1/workspaces',
      store: 'crowbar_workspaces',
      seed,
    })
    await vi.waitFor(async () => {
      expect(await getAllEntities<WorkspaceDTO>('crowbar_workspaces')).toHaveLength(1)
    })

    // The reconnect sentinel must trigger a re-seed (full GET), NOT be upserted
    // as a DTO (which would write a bogus { reconnected: true } record).
    emit({ reconnected: true })

    await vi.waitFor(async () => {
      const all = await getAllEntities<WorkspaceDTO>('crowbar_workspaces')
      expect(all).toHaveLength(2)
      // no corrupt entity (the sentinel has no id, so it must never be stored)
      expect(all.every((w) => w.id === 'w1' || w.id === 'w2')).toBe(true)
    })
    expect(seed).toHaveBeenCalledTimes(2)
  })

  it('unsubscribes from the underlying wsManager channel', async () => {
    const seed = vi.fn(async () => [])
    const dispose = subscribeEntityStream<WorkspaceDTO>({
      endpoint: '/v0/projects/p1/repos/r1/workspaces',
      store: 'crowbar_workspaces',
      seed,
    })
    expect(subscribe).toHaveBeenCalledTimes(1)
    dispose()
    expect(unsubscribe).toHaveBeenCalledTimes(1)
  })
})
