import { beforeEach, describe, expect, it } from 'vitest'
import {
  getInitialRemovalState,
  useRemovalTrayStore,
  type RemovalDraft,
} from '@/lib/store/sidebar-removal'

/**
 * `hiddenIds` and `entries` do not end together (sidebar-removal.ts's own
 * module doc): a committed entry leaves `entries` the instant its clock (or
 * an explicit commit) fires, but its rows must stay in `hiddenIds` until the
 * daemon's tombstones actually arrive — `release()` is the only thing that
 * may clear them. `settle()` dropping an id early is exactly the ghost-row
 * bug: a row falls out of the tray's `entries` before its DELETE resolves,
 * with nothing left hiding it.
 */

function draft(over: Partial<RemovalDraft> = {}): RemovalDraft {
  return {
    kind: 'workspace',
    id: 'ws-1',
    label: 'feature/one',
    projectId: 'p1',
    repoId: 'r1',
    wsId: '',
    providerIcon: '',
    hiddenIds: ['ws-1', 'chat-1'],
    extra: 0,
    fallbackWsId: null,
    ...over,
  }
}

beforeEach(() => {
  useRemovalTrayStore.setState(getInitialRemovalState())
})

describe('hold', () => {
  it('hides every id the draft names, primary included', () => {
    useRemovalTrayStore.getState().hold([draft()])

    const hiddenIds = useRemovalTrayStore.getState().hiddenIds
    expect(hiddenIds.has('ws-1')).toBe(true)
    expect(hiddenIds.has('chat-1')).toBe(true)
  })
})

describe('settle', () => {
  it('drops the tray row but keeps every one of its ids hidden', () => {
    useRemovalTrayStore.getState().hold([draft()])
    const [entry] = useRemovalTrayStore.getState().entries

    useRemovalTrayStore.getState().settle(entry.entryId)

    expect(useRemovalTrayStore.getState().entries).toHaveLength(0)
    const hiddenIds = useRemovalTrayStore.getState().hiddenIds
    expect(hiddenIds.has('ws-1')).toBe(true)
    expect(hiddenIds.has('chat-1')).toBe(true)
  })

  it('is a no-op on hiddenIds for an entry id nothing holds', () => {
    const before = useRemovalTrayStore.getState().hiddenIds
    useRemovalTrayStore.getState().settle('not-a-real-entry')
    expect(useRemovalTrayStore.getState().hiddenIds).toBe(before)
  })
})

describe('release', () => {
  it('only THIS clears settled ids back out of hiddenIds', () => {
    useRemovalTrayStore.getState().hold([draft()])
    const [entry] = useRemovalTrayStore.getState().entries
    useRemovalTrayStore.getState().settle(entry.entryId)
    expect(useRemovalTrayStore.getState().hiddenIds.has('ws-1')).toBe(true)

    useRemovalTrayStore.getState().release(entry.hiddenIds)

    const hiddenIds = useRemovalTrayStore.getState().hiddenIds
    expect(hiddenIds.has('ws-1')).toBe(false)
    expect(hiddenIds.has('chat-1')).toBe(false)
  })
})

describe('cancel', () => {
  it('drops both the tray row and its hidden ids at once — the undo path, unlike settle', () => {
    useRemovalTrayStore.getState().hold([draft()])
    const [entry] = useRemovalTrayStore.getState().entries

    useRemovalTrayStore.getState().cancel(entry.entryId)

    expect(useRemovalTrayStore.getState().entries).toHaveLength(0)
    expect(useRemovalTrayStore.getState().hiddenIds.has('ws-1')).toBe(false)
    expect(useRemovalTrayStore.getState().hiddenIds.has('chat-1')).toBe(false)
  })
})

describe('askToDiscard', () => {
  it('puts a sent entry back, waiting on an answer with the work at risk, its rows still hidden', () => {
    useRemovalTrayStore.getState().hold([draft()])
    const [sent] = useRemovalTrayStore.getState().entries
    useRemovalTrayStore.getState().settle(sent.entryId)
    const atRisk = [
      { workspaceId: 'ws-1', branch: 'feature/one', uncommittedFiles: 1, unmergedCommits: 0 },
    ]

    useRemovalTrayStore.getState().askToDiscard(sent, atRisk)

    const [asked] = useRemovalTrayStore.getState().entries
    expect(asked.deadlineAt).toBeNull()
    expect(asked.atRisk).toEqual(atRisk)
    expect(asked.entryId).not.toBe(sent.entryId)
    expect(useRemovalTrayStore.getState().hiddenIds.has('chat-1')).toBe(true)

    useRemovalTrayStore.getState().cancel(asked.entryId)
    expect(useRemovalTrayStore.getState().hiddenIds.has('ws-1')).toBe(false)
  })
})
