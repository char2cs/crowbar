import { describe, it, expect, beforeEach } from 'vitest'
import {
  setActiveWorkspaceStoreRef,
  getActiveWorkspaceStoreRef,
  onActiveWorkspaceStoreChange,
} from '@/features/workspace/stores/workspace-store-ref'
import type { WorkspaceStore } from '@/features/workspace/stores/workspace-store'

function fakeStore(id: string): WorkspaceStore {
  return { id } as unknown as WorkspaceStore
}

beforeEach(() => {
  // Reset module state between tests by setting null without triggering listeners
  setActiveWorkspaceStoreRef(null)
})

describe('workspace-store-ref null window', () => {
  it('does not fire null when a new store is set in the same sync tick', async () => {
    const storeA = fakeStore('a')
    const storeB = fakeStore('b')

    // Register and consume the immediate fire
    const calls: Array<WorkspaceStore | null> = []
    const unsub = onActiveWorkspaceStoreChange((s) => calls.push(s))
    calls.length = 0 // discard the immediate-fire null

    // Simulate workspace switch: old unmounts (null), new mounts (storeB) in same tick
    setActiveWorkspaceStoreRef(storeA)
    setActiveWorkspaceStoreRef(null)
    setActiveWorkspaceStoreRef(storeB)

    await Promise.resolve() // drain microtask queue

    expect(calls).not.toContain(null)
    expect(calls[calls.length - 1]).toBe(storeB)

    unsub()
  })

  it('fires null when no new store follows', async () => {
    const storeA = fakeStore('a')

    const calls: Array<WorkspaceStore | null> = []
    const unsub = onActiveWorkspaceStoreChange((s) => calls.push(s))
    calls.length = 0

    setActiveWorkspaceStoreRef(storeA)
    setActiveWorkspaceStoreRef(null) // last workspace closed

    await Promise.resolve()

    expect(calls).toContain(null)

    unsub()
  })

  it('getActiveWorkspaceStoreRef returns current store', () => {
    const store = fakeStore('x')
    setActiveWorkspaceStoreRef(store)
    expect(getActiveWorkspaceStoreRef()).toBe(store)
    setActiveWorkspaceStoreRef(null)
    expect(getActiveWorkspaceStoreRef()).toBeNull()
  })
})
