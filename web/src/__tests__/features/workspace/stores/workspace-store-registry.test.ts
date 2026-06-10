import { describe, it, expect, afterEach } from 'vitest'
import {
  getOrCreateWorkspaceStore,
  destroyWorkspaceStore,
  getAllActiveWorkspaceIds,
} from '@/features/workspace/stores/workspace-store-registry'

afterEach(() => {
  getAllActiveWorkspaceIds().forEach((id) => destroyWorkspaceStore(id))
})

describe('workspace-store-registry', () => {
  it('getOrCreate returns the same instance for the same wsId', () => {
    const a = getOrCreateWorkspaceStore('ws-a')
    const b = getOrCreateWorkspaceStore('ws-a')
    expect(a).toBe(b)
  })

  it('getOrCreate returns different instances for different wsIds', () => {
    const a = getOrCreateWorkspaceStore('ws-x')
    const b = getOrCreateWorkspaceStore('ws-y')
    expect(a).not.toBe(b)
  })

  it('destroyWorkspaceStore removes the instance', () => {
    const first = getOrCreateWorkspaceStore('ws-z')
    destroyWorkspaceStore('ws-z')
    const second = getOrCreateWorkspaceStore('ws-z')
    expect(first).not.toBe(second)
  })

  it('getAllActiveWorkspaceIds returns ids of live stores', () => {
    getOrCreateWorkspaceStore('ws-1')
    getOrCreateWorkspaceStore('ws-2')
    const ids = getAllActiveWorkspaceIds()
    expect(ids).toContain('ws-1')
    expect(ids).toContain('ws-2')
  })
})
