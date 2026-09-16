import { describe, it, expect, afterEach, vi } from 'vitest'
import {
  getOrCreateWorkspaceStore,
  getWorkspaceStore,
  destroyWorkspaceStore,
  getAllActiveWorkspaceIds,
  resolveWorkspaceIdForChat,
  setActiveWorkspaceId,
  clearActiveWorkspaceId,
  getActiveWorkspaceId,
  subscribeWorkspaceStores,
  subscribeChatWorking,
  readChatWorking,
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
// Fake Monaco adapters so armEditor() (dynamically imported by both
// WorkspaceStore itself and destroyWorkspaceStore's teardown) never touches
// real monaco-editor — same fakes editor-pane-workspace-scope.test.tsx uses.
vi.mock('@/features/editor/lib/monaco-adapters', () => ({
  EDITOR_CREATE_OPTIONS: {},
  langForUri: () => 'plaintext',
  realModelApi: () => ({
    createModel: (value: string, _lang: string, uri: string) => ({
      uri,
      dispose: () => {},
      getValue: () => value,
      setValueIfChanged: () => {},
    }),
    getModel: () => null,
  }),
  realEditorApi: () => ({
    create: () => ({
      setModel: () => {},
      getModel: () => null,
      saveViewState: () => null,
      restoreViewState: () => {},
      layout: () => {},
      dispose: () => {},
      raw: () => null,
    }),
  }),
}))

afterEach(() => {
  getAllActiveWorkspaceIds().forEach((id) => destroyWorkspaceStore(id))
  const active = getActiveWorkspaceId()
  if (active) clearActiveWorkspaceId(active)
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

  // Regression: planRetention (keep-alive-policy.ts) evicts purely off
  // hasViewChat/RETENTION_CAP — it has no notion of "a pane's editor tab
  // still needs this workspace" — so destroyWorkspaceStore can be called
  // for a workspace whose EditorManager still has a widget mounted into a
  // real pane. The disposeAll() gate below already protected the manager's
  // OWN resources in that case, but registry.delete(wsId) ran regardless,
  // making the store unreachable via getWorkspaceStore anyway — the next
  // render of that pane's EditorSurface saw "no store" and fell back to the
  // ambient workspace, remounting the retained widget onto the WRONG
  // manager and landing on a silently empty model. Live-reported as a
  // blank editor pane with no console error and no repro steps.
  it('does not evict a workspace whose EditorManager still has a mounted pane', async () => {
    const store = getOrCreateWorkspaceStore('ws-mounted-pane')
    await store.armEditor()
    store.editorManager!.mountPane('pane-1', document.createElement('div'))

    destroyWorkspaceStore('ws-mounted-pane')

    expect(getWorkspaceStore('ws-mounted-pane')).toBe(store)
    expect(getAllActiveWorkspaceIds()).toContain('ws-mounted-pane')

    // Once the pane's widget actually unmounts (its editor tab closes), the
    // SAME call must go through normally.
    store.editorManager!.unmountPane('pane-1')
    destroyWorkspaceStore('ws-mounted-pane')
    expect(getWorkspaceStore('ws-mounted-pane')).toBeUndefined()
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

  // `getOrCreateWorkspaceStore` is called FROM THE RENDER PATH
  // (WorkspaceView/WindowPaneSurface mint the store they provide as context),
  // so a registration that pushes a change at its watchers pushes a setState
  // out of React's render phase — live-observed as "Cannot update a component
  // (`IDEShell`) while rendering a different component (`WorkspaceView`)".
  // A brand-new store has no agentChats, so it can move no watcher's answer;
  // the re-bind still has to be synchronous, because the next write to that
  // very store (its chats stream landing) is what carries the real change.
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

      getOrCreateWorkspaceStore('ws-fresh').getState().upsertAgentChat(chat('chat-1', 'ws-fresh'))

      expect(fired).toHaveBeenCalled()
      unsubscribe()
    })

    it('fires watchers when a store is destroyed — that really does change the answer', () => {
      getOrCreateWorkspaceStore('ws-doomed').getState().upsertAgentChat(chat('chat-1', 'ws-doomed'))
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

      getOrCreateWorkspaceStore('ws-late').getState().setAgentChatWorking('chat-late', true)
      expect(fired).toHaveBeenCalled()
      expect(readChatWorking('ws-late', 'chat-late')).toBe(true)

      unsubscribe()
    })
  })

  // Live-reported: file-explorer state (and anything else keyed off
  // getWorkspaceScope()'s active id) for a chat sharing a workspace with
  // sibling chats kept reading/writing a DIFFERENT workspace than the one
  // actually on screen. Root cause: WorkspaceView's active-only effect
  // called setActiveWorkspaceId(wsId) with no cleanup at all — unlike its
  // sibling setActiveWorkspaceStoreRef effect right above it, which does
  // null itself out on deactivation — so the id kept pointing at a
  // workspace whose WorkspaceView had since unmounted (evicted from
  // WorkspaceHost's retention), a dangling reference nothing ever corrected
  // for a workspace with no dedicated route of its own to re-claim it.
  describe('setActiveWorkspaceId / clearActiveWorkspaceId', () => {
    it('clearActiveWorkspaceId resets the active id when it is still the one recorded', () => {
      setActiveWorkspaceId('ws-a')
      expect(getActiveWorkspaceId()).toBe('ws-a')

      clearActiveWorkspaceId('ws-a')

      expect(getActiveWorkspaceId()).toBeNull()
    })

    it('clearActiveWorkspaceId is a no-op once a different workspace has claimed the id', () => {
      setActiveWorkspaceId('ws-a')
      setActiveWorkspaceId('ws-b') // ws-b's WorkspaceView became active first

      // ws-a's own effect cleanup fires afterward (its `active` flipped
      // false, or it unmounted) — it must not clobber ws-b's newer claim.
      clearActiveWorkspaceId('ws-a')

      expect(getActiveWorkspaceId()).toBe('ws-b')
    })
  })
})
