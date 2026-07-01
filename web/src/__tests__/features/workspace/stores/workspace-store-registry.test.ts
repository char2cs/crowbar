import { describe, it, expect, afterEach, vi, beforeEach } from 'vitest'
import {
  getOrCreateWorkspaceStore,
  destroyWorkspaceStore,
  getAllActiveWorkspaceIds,
} from '@/features/workspace/stores/workspace-store-registry'

// Mock the IDB-backed persistence so tests don't need a real IndexedDB write path
// and so we can assert exactly when saveWorkspaceLayout is called.
vi.mock('@/lib/persistence/workspace-layout', () => ({
  saveWorkspaceLayout: vi.fn().mockResolvedValue(undefined),
}))

// Mock the session writer (the store's INNER subscription) so we can assert it
// stops firing once the store is destroyed.
vi.mock('@/features/editor/stores/buffer-session-persistence', () => ({
  saveSessionToStore: vi.fn(),
  clearQueuedWorkspaceSessionSave: vi.fn(),
}))

import { saveWorkspaceLayout } from '@/lib/persistence/workspace-layout'
import { saveSessionToStore } from '@/features/editor/stores/buffer-session-persistence'
const mockSave = saveWorkspaceLayout as ReturnType<typeof vi.fn>
const mockSaveSession = saveSessionToStore as ReturnType<typeof vi.fn>

afterEach(() => {
  getAllActiveWorkspaceIds().forEach((id) => destroyWorkspaceStore(id))
  vi.restoreAllMocks()
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

describe('workspace-store-registry persistence subscription', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    mockSave.mockClear()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('(a) destroyWorkspaceStore unsubscribes — no persistence fires after destroy', () => {
    const store = getOrCreateWorkspaceStore('ws-destroy-a')

    // Trigger a persisted-field mutation to arm the debounce timer, then destroy.
    store.getState().paneActions.setActivePane('some-pane-id')

    // Destroy before the debounce fires — should clear the timer and unsubscribe.
    destroyWorkspaceStore('ws-destroy-a')

    // Advance past the debounce window; timer was cleared so save must NOT fire.
    vi.advanceTimersByTime(400)
    expect(mockSave).not.toHaveBeenCalled()

    // Recreate the store and trigger another persisted-field mutation.
    const store2 = getOrCreateWorkspaceStore('ws-destroy-a')
    store2.getState().paneActions.setActivePane('another-pane-id')

    // Destroy again immediately — no save should fire.
    destroyWorkspaceStore('ws-destroy-a')
    vi.advanceTimersByTime(400)
    expect(mockSave).not.toHaveBeenCalled()
  })

  it('(b) non-persisted field mutation does NOT trigger a persistence write', () => {
    const store = getOrCreateWorkspaceStore('ws-nonpersisted-b')

    // Mutate a non-persisted field (LSP status — not in WorkspaceLayout schema).
    store.getState().lspActions.updateLspStatus({ status: 'running' })

    // Advance well past the debounce window.
    vi.advanceTimersByTime(400)

    expect(mockSave).not.toHaveBeenCalled()
  })

  it('(b) persisted layout field mutation DOES trigger a persistence write after debounce', () => {
    const store = getOrCreateWorkspaceStore('ws-persisted-b')

    // Mutate a persisted layout field (activePaneId via setActivePane).
    store.getState().paneActions.setActivePane('test-pane-id')

    // Save should NOT have been called yet (debounce pending).
    expect(mockSave).not.toHaveBeenCalled()

    // Advance past the 300 ms debounce window.
    vi.advanceTimersByTime(400)

    expect(mockSave).toHaveBeenCalledTimes(1)
    expect(mockSave).toHaveBeenCalledWith(
      expect.objectContaining({ workspaceId: 'ws-persisted-b' }),
    )
  })

  it('(b) non-persisted mutation followed by persisted mutation only saves once after debounce', () => {
    const store = getOrCreateWorkspaceStore('ws-mixed-b')

    // Non-persisted mutation — must not arm the timer.
    store.getState().lspActions.updateLspStatus({ status: 'running' })

    // Persisted mutation — must arm the timer.
    store.getState().paneActions.setActivePane('target-pane')

    // Another non-persisted mutation — must not re-arm or duplicate.
    store.getState().lspActions.updateLspStatus({ status: 'idle' })

    vi.advanceTimersByTime(400)

    // Exactly one save for the one persisted change (timer was debounced).
    expect(mockSave).toHaveBeenCalledTimes(1)
  })
})

describe('workspace-store inner session subscription (pass-3 #2)', () => {
  beforeEach(() => {
    mockSaveSession.mockClear()
  })

  it('destroyWorkspaceStore disposes the inner session writer — no session write after destroy', () => {
    const store = getOrCreateWorkspaceStore('ws-session')

    // A buffers-identity change fires the inner session writer.
    store.setState({ buffers: [...store.getState().buffers] })
    expect(mockSaveSession).toHaveBeenCalled()

    mockSaveSession.mockClear()
    destroyWorkspaceStore('ws-session')

    // A late setState on the destroyed store must NOT write a stale session.
    store.setState({ buffers: [...store.getState().buffers] })
    expect(mockSaveSession).not.toHaveBeenCalled()
  })
})
