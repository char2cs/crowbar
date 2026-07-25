import { describe, it, expect } from 'vitest'
import { stripNewTabs, type Snapshot } from '@/features/workspace/stores/persisted-layout'

describe('stripNewTabs', () => {
  const snapshot = {
    buffers: [
      { id: 'nt-1', type: 'newTab', path: '', name: 'New Tab' },
      { id: 'e-1', type: 'editor', path: '/a.ts', name: 'a.ts' },
    ],
    panes: {
      root: { id: 'root', bufferIds: ['nt-1', 'e-1'], activeBufferId: 'nt-1' },
      split: { id: 'split', bufferIds: ['nt-2'], activeBufferId: 'nt-2' },
    },
  } as unknown as Snapshot

  it('drops newTab buffers', () => {
    expect(stripNewTabs(snapshot).buffers.map((b) => b.id)).toEqual(['e-1'])
  })

  it('strips their ids out of pane membership, so no id is left stranded', () => {
    expect(stripNewTabs(snapshot).panes.root.bufferIds).toEqual(['e-1'])
  })

  it('repoints activeBufferId when it pointed at a New Tab', () => {
    const out = stripNewTabs(snapshot)
    expect(out.panes.root.activeBufferId).toBe('e-1')
    // Nothing left to activate — null, never a dangling id.
    expect(out.panes.split.activeBufferId).toBeNull()
  })

  it("drops a New Tab that is a pane's only tab, leaving it empty", () => {
    const soleNewTab = {
      buffers: [{ id: 'nt-only', type: 'newTab', path: '', name: 'New Tab' }],
      panes: {
        root: { id: 'root', bufferIds: ['nt-only'], activeBufferId: 'nt-only' },
      },
    } as unknown as Snapshot

    const out = stripNewTabs(soleNewTab)
    expect(out.buffers.map((b) => b.id)).toEqual([])
    expect(out.panes.root.bufferIds).toEqual([])
    expect(out.panes.root.activeBufferId).toBeNull()
  })

  it('heals a pre-existing stranded id even when no New Tab is open (Finding 1)', () => {
    // No newTab buffers at all — the guard must still fire the heal, not bail
    // out early, since a pane holds an id with no backing buffer.
    const strandedOnly = {
      buffers: [{ id: 'e-1', type: 'editor', path: '/a.ts', name: 'a.ts' }],
      panes: {
        root: { id: 'root', bufferIds: ['e-1', 'ghost'], activeBufferId: 'ghost' },
      },
    } as unknown as Snapshot

    const out = stripNewTabs(strandedOnly)
    expect(out.panes.root.bufferIds).toEqual(['e-1'])
    expect(out.panes.root.activeBufferId).toBe('e-1')
  })

  it('returns the same snapshot reference when there is truly nothing to strip', () => {
    const clean = {
      buffers: [{ id: 'e-1', type: 'editor', path: '/a.ts', name: 'a.ts' }],
      panes: {
        root: { id: 'root', bufferIds: ['e-1'], activeBufferId: 'e-1' },
      },
    } as unknown as Snapshot

    expect(stripNewTabs(clean)).toBe(clean)
  })
})
