import { describe, it, expect, afterEach, vi } from 'vitest'
import {
  getOrCreateWorkspaceStore,
  getWorkspaceStore,
  destroyWorkspaceStore,
  getAllActiveWorkspaceIds,
  resolveWorkspaceIdForChat,
} from '@/features/workspace/stores/workspace-store-registry'
import type { AgentChat } from '@/features/agent/api/agent-api'

const chat = (id: string, workspaceId: string): AgentChat => ({
  id,
  workspaceId,
  title: id,
  liveRunnerId: '',
  terminalSessionId: '',
  activeProviderId: 'claude',
  createdAt: '2026-01-01T00:00:00Z',
  order: 0,
})

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

  // Task 27: the chatId -> workspaceId resolution Task 26's own review found
  // missing from the render path entirely. Mirrors isChatWorking's own
  // real-store-via-the-registry test style rather than mocking the scan.
  describe('resolveWorkspaceIdForChat', () => {
    it('returns the id of the registered store whose agentChats.chats names the chat', () => {
      const store = getOrCreateWorkspaceStore('ws-a')
      store.getState().upsertAgentChat(chat('chat-1', 'ws-a'))
      expect(resolveWorkspaceIdForChat('chat-1')).toBe('ws-a')
    })

    it('searches every registered store, not just the first', () => {
      getOrCreateWorkspaceStore('ws-a').getState().upsertAgentChat(chat('chat-a', 'ws-a'))
      const storeB = getOrCreateWorkspaceStore('ws-b')
      storeB.getState().upsertAgentChat(chat('chat-b', 'ws-b'))
      expect(resolveWorkspaceIdForChat('chat-b')).toBe('ws-b')
    })

    it('returns null when no registered store names the chat', () => {
      getOrCreateWorkspaceStore('ws-a').getState().upsertAgentChat(chat('chat-1', 'ws-a'))
      expect(resolveWorkspaceIdForChat('chat-never-seen')).toBeNull()
    })

    it('returns null when nothing is registered at all', () => {
      expect(resolveWorkspaceIdForChat('chat-1')).toBeNull()
    })

    it('stops naming a chat once its owning store is destroyed', () => {
      getOrCreateWorkspaceStore('ws-a').getState().upsertAgentChat(chat('chat-1', 'ws-a'))
      expect(resolveWorkspaceIdForChat('chat-1')).toBe('ws-a')
      destroyWorkspaceStore('ws-a')
      expect(resolveWorkspaceIdForChat('chat-1')).toBeNull()
    })

    it('resolves the workspace that actually owns the chat, not the caller-active one', () => {
      // The whole point of the resolver (Task 26's own review): a chat's
      // owning workspace has to be found on its own terms, independent of
      // whichever workspace happens to be globally "active" elsewhere.
      getOrCreateWorkspaceStore('ws-active').getState().upsertAgentChat(chat('chat-x', 'ws-active'))
      getOrCreateWorkspaceStore('ws-background')
        .getState()
        .upsertAgentChat(chat('chat-y', 'ws-background'))
      expect(resolveWorkspaceIdForChat('chat-y')).toBe('ws-background')
    })

    it("agrees with the chat record's own workspaceId in the ordinary (non-evicted) case", () => {
      // Documents the doc comment's claim: the registry key and the chat's
      // own denormalized `workspaceId` field are expected to agree whenever
      // the owning store is actually registered — this resolver just never
      // relies on the denormalized field to make that true.
      const record = chat('chat-1', 'ws-a')
      getOrCreateWorkspaceStore('ws-a').getState().upsertAgentChat(record)
      expect(resolveWorkspaceIdForChat('chat-1')).toBe(record.workspaceId)
    })

    // Fix round 1 (coordinator review): the resolver's PRIMARY intended
    // case — Task 26 deliberately hoisted panes to window level so a pane
    // holding a chat OUTLIVES its owning workspace's own eviction
    // (WorkspaceHost's age/LRU keep-alive window; see workspace-host.tsx).
    // "Registered stores only" therefore means the one scenario this
    // resolver exists to serve — a pane whose chat's workspace has since
    // been evicted — is exactly the case where it answers null. This is
    // documented as a deliberate characteristic on the function itself
    // (REGISTRY-SCOPED, NOT OMNISCIENT), not a silent gap; this test pins
    // that characteristic down so a future change can't quietly alter it.
    it('resolves to null for a chat whose workspace was evicted, even though a pane can still reference it', () => {
      getOrCreateWorkspaceStore('ws-evicted')
        .getState()
        .upsertAgentChat(chat('chat-1', 'ws-evicted'))
      expect(resolveWorkspaceIdForChat('chat-1')).toBe('ws-evicted')

      // WorkspaceHost's own eviction path: destroy the store, exactly as it
      // does when a workspace ages out of the keep-alive window. Nothing
      // about the pane that still holds `chat-1` changes here — panes are
      // window-level and outlive this by design (Task 26).
      destroyWorkspaceStore('ws-evicted')

      expect(resolveWorkspaceIdForChat('chat-1')).toBeNull()
    })
  })
})
