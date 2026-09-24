import { describe, it, expect, afterEach, vi } from 'vitest'
import {
  getOrCreateWorkspaceStore,
  getWorkspaceStore,
  destroyWorkspaceStore,
  getAllActiveWorkspaceIds,
  setActiveWorkspaceId,
  canEvictWorkspace,
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
  // The host asks BEFORE letting go (canEvict), so a destroy is never vetoed
  // after the fact — that left a zombie store registered.
  it('a workspace whose EditorManager still has a mounted pane cannot be evicted', async () => {
    const store = getOrCreateWorkspaceStore('ws-mounted-pane')
    await store.armEditor()
    store.editorManager!.mountPane('pane-1', document.createElement('div'))
    expect(canEvictWorkspace('ws-mounted-pane')).toBe(false)

    store.editorManager!.unmountPane('pane-1')
    expect(canEvictWorkspace('ws-mounted-pane')).toBe(true)
    destroyWorkspaceStore('ws-mounted-pane')
    expect(getWorkspaceStore('ws-mounted-pane')).toBeUndefined()
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

  it('the active id is one value the host writes, cleared with null', () => {
    setActiveWorkspaceId('ws-a')
    expect(getActiveWorkspaceId()).toBe('ws-a')
    setActiveWorkspaceId(null)
    expect(getActiveWorkspaceId()).toBeNull()
  })
})
