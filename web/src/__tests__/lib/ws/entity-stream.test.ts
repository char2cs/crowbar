import { describe, it, expect, vi, beforeEach } from 'vitest'
import type { WorkspaceDTO } from '@/lib/types'
import type { EntityStoreName } from '@/lib/persistence/entity-cache'

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

// Mock the IndexedDB cache layer with a controllable in-memory store. The key
// lever is `upsertDelayMs`: an upsert that resolves SLOWLY lets us reproduce the
// H21 race where a fast delete would otherwise commit first. With the serial
// applyChain in entity-stream, arrival order is preserved regardless of latency.
const cache = new Map<string, Map<string, { id: string; status?: string }>>()
let upsertDelayMs = 0

function bucket(store: EntityStoreName): Map<string, { id: string; status?: string }> {
  let b = cache.get(store)
  if (!b) {
    b = new Map()
    cache.set(store, b)
  }
  return b
}

function delay(ms: number): Promise<void> {
  return ms > 0 ? new Promise((r) => setTimeout(r, ms)) : Promise.resolve()
}

vi.mock('@/lib/persistence/entity-cache', () => ({
  upsertEntity: vi.fn(async (store: EntityStoreName, dto: { id: string; status?: string }) => {
    await delay(upsertDelayMs)
    bucket(store).set(dto.id, dto)
  }),
  removeEntity: vi.fn(async (store: EntityStoreName, id: string) => {
    bucket(store).delete(id)
  }),
  getAllEntities: vi.fn(async (store: EntityStoreName) => Array.from(bucket(store).values())),
}))

const { subscribeEntityStream } = await import('@/lib/ws/entity-stream')
const { getAllEntities } = await import('@/lib/persistence/entity-cache')

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
  cache.clear()
  upsertDelayMs = 0
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

  // H21 part 1: ordering. A 'new' frame immediately followed by a 'deleted'
  // tombstone for the SAME id (rapid mutate/abort) must end REMOVED. We make the
  // upsert resolve slowly so an unordered, fire-and-forget implementation would
  // let the fast delete commit first and the late upsert would resurrect a ghost
  // row. The serial applyChain forces the delete to land after the upsert.
  it('keeps an entity REMOVED when upsert(id) is immediately followed by remove(id)', async () => {
    const seed = vi.fn(async () => [])
    subscribeEntityStream<WorkspaceDTO>({
      endpoint: '/v0/projects/p1/repos/r1/workspaces',
      store: 'crowbar_workspaces',
      seed,
    })
    await vi.waitFor(() => expect(seed).toHaveBeenCalled())

    upsertDelayMs = 20 // upsert resolves slowly, delete resolves immediately
    emit(makeWorkspace({ id: 'wx', status: 'new' }))
    emit(makeWorkspace({ id: 'wx', status: 'deleted' }))

    await vi.waitFor(async () => {
      // both frames applied — and the entity is gone, not resurrected
      expect(await getAllEntities<WorkspaceDTO>('crowbar_workspaces')).toHaveLength(0)
    })
    // give any out-of-order late upsert a chance to (wrongly) revive it
    await new Promise((r) => setTimeout(r, 40))
    expect(await getAllEntities<WorkspaceDTO>('crowbar_workspaces')).toHaveLength(0)
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

  // H21 part 2: authoritative reseed. A GET is a point-in-time snapshot; on
  // reconnect any entity deleted DURING the outage is absent from the fresh seed
  // and must be PRUNED from the cache, not left to linger.
  it('reseed prunes a cached entity that is absent from the fresh seed', async () => {
    let seedCalls = 0
    const seed = vi.fn(async () => {
      seedCalls++
      // first seed has w1+w2; after the outage only w1 survives (w2 was deleted)
      return seedCalls === 1
        ? [makeWorkspace({ id: 'w1' }), makeWorkspace({ id: 'w2' })]
        : [makeWorkspace({ id: 'w1' })]
    })
    subscribeEntityStream<WorkspaceDTO>({
      endpoint: '/v0/projects/p1/repos/r1/workspaces',
      store: 'crowbar_workspaces',
      seed,
    })
    await vi.waitFor(async () => {
      expect(await getAllEntities<WorkspaceDTO>('crowbar_workspaces')).toHaveLength(2)
    })

    emit({ reconnected: true })

    await vi.waitFor(async () => {
      const all = await getAllEntities<WorkspaceDTO>('crowbar_workspaces')
      expect(all.map((w) => w.id)).toEqual(['w1'])
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
