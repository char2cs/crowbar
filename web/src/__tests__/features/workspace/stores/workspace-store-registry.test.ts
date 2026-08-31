import { describe, it, expect, afterEach, vi } from 'vitest'
import {
  getOrCreateWorkspaceStore,
  getWorkspaceStore,
  destroyWorkspaceStore,
  getAllActiveWorkspaceIds,
} from '@/features/workspace/stores/workspace-store-registry'

// Mock the IDB-backed persistence so a real destroyWorkspaceStore call (which
// dynamically imports window-pane-store.ts for its buffer-scoped teardown)
// doesn't need a real IndexedDB write path.
vi.mock('@/lib/persistence/workspace-layout', () => ({
  saveWorkspaceLayout: vi.fn().mockResolvedValue(undefined),
}))
vi.mock('@/features/editor/stores/buffer-session-persistence', () => ({
  saveSessionToStore: vi.fn(),
  clearQueuedWorkspaceSessionSave: vi.fn(),
}))

afterEach(() => {
  getAllActiveWorkspaceIds().forEach((id) => destroyWorkspaceStore(id))
  vi.restoreAllMocks()
})

// Task 26: pane/buffer layout moved off this registry onto the window-level
// pane store (window-pane-store.ts), which is created once and never
// destroyed — the "destroyWorkspaceStore unsubscribes the persistence
// writer"/"disposes the inner session writer" tests that used to live here
// moved with it; see window-pane-store.test.ts. What's left to test here is
// the registry's own instance lifecycle, which did not change.
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

  // Fix round 1 (I3): getWorkspaceStore must NEVER mint a store for a
  // workspace nobody registered — editorManagerFor (pane-slice.ts/
  // buffer-slice.ts) uses this to resolve a buffer's Monaco manager by
  // workspaceId, and a buffer can outlive its owning workspace's eviction.
  // The old getOrCreateWorkspaceStore-based lookup would silently
  // re-register (and leak, for the rest of the session) a store
  // WorkspaceHost never mounted and will never destroy.
  it('getWorkspaceStore returns undefined for an unregistered workspace, without creating one', () => {
    expect(getWorkspaceStore('ws-never-registered')).toBeUndefined()
    expect(getAllActiveWorkspaceIds()).not.toContain('ws-never-registered')
  })

  it('getWorkspaceStore returns the same instance getOrCreateWorkspaceStore already made', () => {
    const created = getOrCreateWorkspaceStore('ws-already-there')
    expect(getWorkspaceStore('ws-already-there')).toBe(created)
  })

  it('getWorkspaceStore returns undefined again once the workspace is destroyed', () => {
    getOrCreateWorkspaceStore('ws-evicted')
    destroyWorkspaceStore('ws-evicted')
    expect(getWorkspaceStore('ws-evicted')).toBeUndefined()
  })
})
