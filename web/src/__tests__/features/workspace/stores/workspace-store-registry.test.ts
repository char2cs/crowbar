import { describe, it, expect, afterEach, vi } from 'vitest'
import {
  getOrCreateWorkspaceStore,
  getWorkspaceStore,
  destroyWorkspaceStore,
  getAllActiveWorkspaceIds,
  setActiveWorkspaceId,
  getActiveWorkspaceId,
  subscribeWorkspaceStores,
  subscribeChatWorking,
  readChatWorking,
} from '@/features/workspace/stores/workspace-store-registry'
import { setChatWorking, writeChat } from '@/__tests__/__fixtures__/agent-chat'

afterEach(() => {
  getAllActiveWorkspaceIds().forEach((id) => destroyWorkspaceStore(id))
  setActiveWorkspaceId(null)
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

  // getWorkspaceStore never mints a store: a buffer can outlive its
  // workspace's eviction, and a store minted for its id would leak.
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

  describe('registry change notifications', () => {
    it('does not fire watchers when a store is merely registered', () => {
      const fired = vi.fn()
      const unsubscribe = subscribeWorkspaceStores(fired)

      getOrCreateWorkspaceStore('ws-fresh')

      expect(fired).not.toHaveBeenCalled()
      unsubscribe()
    })

    it('still binds that new store, so its very next write DOES fire them', () => {
      const fired = vi.fn()
      const unsubscribe = subscribeWorkspaceStores(fired)

      writeChat(getOrCreateWorkspaceStore('ws-fresh'), { id: 'chat-1', workspaceId: 'ws-fresh' })

      expect(fired).toHaveBeenCalled()
      unsubscribe()
    })

    it('fires watchers when a store is destroyed — that really does change the answer', () => {
      writeChat(getOrCreateWorkspaceStore('ws-doomed'), { id: 'chat-1', workspaceId: 'ws-doomed' })
      const fired = vi.fn()
      const unsubscribe = subscribeWorkspaceStores(fired)

      destroyWorkspaceStore('ws-doomed')

      expect(fired).toHaveBeenCalled()
      unsubscribe()
    })

    it('subscribeChatWorking follows the same rule: silent on registration, live on the write', () => {
      const fired = vi.fn()
      const unsubscribe = subscribeChatWorking('ws-late', fired)

      getOrCreateWorkspaceStore('ws-late')
      expect(fired).not.toHaveBeenCalled()

      setChatWorking(getOrCreateWorkspaceStore('ws-late'), 'chat-late', true)
      expect(fired).toHaveBeenCalled()
      expect(readChatWorking('ws-late', 'chat-late')).toBe(true)

      unsubscribe()
    })
  })

  it('the active id is one value the host writes, cleared with null', () => {
    setActiveWorkspaceId('ws-a')
    expect(getActiveWorkspaceId()).toBe('ws-a')
    setActiveWorkspaceId(null)
    expect(getActiveWorkspaceId()).toBeNull()
  })
})
