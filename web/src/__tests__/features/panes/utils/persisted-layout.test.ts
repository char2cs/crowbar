import { describe, it, expect } from 'vitest'
import { stripNewTabs, type Snapshot } from '@/features/panes/utils/persisted-layout'

describe('stripNewTabs', () => {
  const snapshot = {
    buffers: [
      { id: 'nt-1', type: 'newTab', path: '', name: 'New Tab' },
      { id: 'e-1', type: 'editor', path: '/a.ts', name: 'a.ts' },
    ],
    panes: {
      root: { id: 'root', editorTabIds: ['nt-1', 'e-1'], activeEditorTabId: 'nt-1' },
      split: { id: 'split', editorTabIds: ['nt-2'], activeEditorTabId: 'nt-2' },
    },
  } as unknown as Snapshot

  it('drops buffers whose content type no longer exists (e.g. the retired New Tab placeholder)', () => {
    expect(stripNewTabs(snapshot).buffers.map((b) => b.id)).toEqual(['e-1'])
  })

  it('strips their ids out of pane membership, so no id is left stranded', () => {
    expect(stripNewTabs(snapshot).panes.root.editorTabIds).toEqual(['e-1'])
  })

  it('repoints activeEditorTabId when it pointed at a dropped buffer', () => {
    const out = stripNewTabs(snapshot)
    expect(out.panes.root.activeEditorTabId).toBe('e-1')
    // Nothing left to activate — null, never a dangling id.
    expect(out.panes.split.activeEditorTabId).toBeNull()
  })

  it("drops a buffer that is a pane's only tab, leaving it empty", () => {
    const soleUnknown = {
      buffers: [{ id: 'nt-only', type: 'newTab', path: '', name: 'New Tab' }],
      panes: {
        root: { id: 'root', editorTabIds: ['nt-only'], activeEditorTabId: 'nt-only' },
      },
    } as unknown as Snapshot

    const out = stripNewTabs(soleUnknown)
    expect(out.buffers.map((b) => b.id)).toEqual([])
    expect(out.panes.root.editorTabIds).toEqual([])
    expect(out.panes.root.activeEditorTabId).toBeNull()
  })

  it('heals a pre-existing stranded id even when nothing was dropped (Finding 1)', () => {
    // No unknown-type buffers at all — the guard must still fire the heal,
    // not bail out early, since a pane holds an id with no backing buffer.
    const strandedOnly = {
      buffers: [{ id: 'e-1', type: 'editor', path: '/a.ts', name: 'a.ts' }],
      panes: {
        root: { id: 'root', editorTabIds: ['e-1', 'ghost'], activeEditorTabId: 'ghost' },
      },
    } as unknown as Snapshot

    const out = stripNewTabs(strandedOnly)
    expect(out.panes.root.editorTabIds).toEqual(['e-1'])
    expect(out.panes.root.activeEditorTabId).toBe('e-1')
  })

  it('returns the same snapshot reference when there is truly nothing to strip', () => {
    const clean = {
      buffers: [{ id: 'e-1', type: 'editor', path: '/a.ts', name: 'a.ts' }],
      panes: {
        root: { id: 'root', editorTabIds: ['e-1'], activeEditorTabId: 'e-1' },
      },
    } as unknown as Snapshot

    expect(stripNewTabs(clean)).toBe(clean)
  })

  it('drops a buffer whose content type this build no longer has', () => {
    // A saved layout outlives the code that wrote it. `diff` was the old Monaco
    // commit-diff tab; a layout written before it was retired restores into a
    // renderer that is gone, and the tab paints blank with nothing to say it is
    // stale rather than broken. Graceful fallback, deliberately not migration.
    const stale = {
      buffers: [
        { id: 'old-1', type: 'diff', path: 'diff://commit/abc/all-files', name: 'Commit abc' },
        { id: 'e-1', type: 'editor', path: '/a.ts', name: 'a.ts' },
      ],
      panes: {
        root: { id: 'root', editorTabIds: ['old-1', 'e-1'], activeEditorTabId: 'old-1' },
      },
    } as unknown as Snapshot

    const out = stripNewTabs(stale)

    expect(out.buffers.map((b) => b.id)).toEqual(['e-1'])
    // The pane must not be left pointing at the id that just vanished.
    expect(out.panes.root.editorTabIds).toEqual(['e-1'])
    expect(out.panes.root.activeEditorTabId).toBe('e-1')
  })

  it('keeps every type this build DOES know, including the new commit diff', () => {
    const current = {
      buffers: [
        { id: 'c-1', type: 'commitDiff', wsId: 'w1', sha: 'abc1234', name: 'Commit abc1234' },
        { id: 'br-1', type: 'branchReview', wsId: 'w1', name: 'Branch Review' },
      ],
      panes: {
        root: { id: 'root', editorTabIds: ['c-1', 'br-1'], activeEditorTabId: 'c-1' },
      },
    } as unknown as Snapshot

    expect(stripNewTabs(current)).toBe(current)
  })
})
