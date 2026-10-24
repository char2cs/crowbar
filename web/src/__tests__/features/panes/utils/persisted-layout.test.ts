import { describe, it, expect } from 'vitest'
import {
  toPersistedBuffers,
  validateLoadedBuffers,
  type Snapshot,
} from '@/features/panes/utils/persisted-layout'

describe('validateLoadedBuffers', () => {
  const ROOT = { adoptInto: 'root' }
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
    expect(validateLoadedBuffers(snapshot, ROOT).buffers.map((b) => b.id)).toEqual(['e-1'])
  })

  it('strips their ids out of pane membership, so no id is left stranded', () => {
    expect(validateLoadedBuffers(snapshot, ROOT).panes.root.editorTabIds).toEqual(['e-1'])
  })

  it('repoints activeEditorTabId when it pointed at a dropped buffer', () => {
    const out = validateLoadedBuffers(snapshot, ROOT)
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

    const out = validateLoadedBuffers(soleUnknown, ROOT)
    expect(out.buffers.map((b) => b.id)).toEqual([])
    expect(out.panes.root.editorTabIds).toEqual([])
    expect(out.panes.root.activeEditorTabId).toBeNull()
  })

  describe('browser tabs', () => {
    const browser = (id: string, extra: object = {}) => ({
      id,
      type: 'browser',
      path: `browser://${id}`,
      name: 'Example',
      url: 'https://a.com',
      title: 'Example Domain',
      workspaceId: 'w1',
      ...extra,
    })
    const layoutOf = (...ids: string[]) =>
      ({
        buffers: [...ids.map((id) => browser(id)), { id: 'e-1', type: 'editor', path: '/a.ts' }],
        panes: {
          root: { id: 'root', editorTabIds: [...ids, 'e-1'], activeEditorTabId: ids[0] },
        },
      }) as unknown as Snapshot

    it('round-trips url and title, and never carries a page load across a restart', () => {
      const live = {
        ...layoutOf('b-1'),
        buffers: [browser('b-1', { loading: true }), { id: 'e-1', type: 'editor', path: '/a.ts' }],
      } as unknown as Snapshot

      const saved = toPersistedBuffers(live.buffers)
      expect(saved[0]).not.toHaveProperty('loading')

      const out = validateLoadedBuffers({ ...live, buffers: saved }, ROOT)
      expect(out.buffers[0]).toMatchObject({
        id: 'b-1',
        url: 'https://a.com',
        title: 'Example Domain',
      })
      expect(out.panes.root.editorTabIds).toEqual(['b-1', 'e-1'])
    })

    it('restores as not loading even from a record that stored loading: true', () => {
      const stored = {
        ...layoutOf('b-1'),
        buffers: [browser('b-1', { loading: true })],
      } as unknown as Snapshot
      expect(validateLoadedBuffers(stored, ROOT).buffers[0]).not.toHaveProperty('loading')
    })

    it('drops a tab whose id could not name a native webview, repairing its pane', () => {
      const out = validateLoadedBuffers(layoutOf('good_id-1', 'bad id!', 'a.b'), ROOT)
      expect(out.buffers.map((b) => b.id)).toEqual(['good_id-1', 'e-1'])
      expect(out.panes.root.editorTabIds).toEqual(['good_id-1', 'e-1'])
    })

    it('drops a tab with no usable url field', () => {
      const broken = { ...layoutOf('b-1'), buffers: [browser('b-1', { url: undefined })] }
      const out = validateLoadedBuffers(broken as unknown as Snapshot, ROOT)
      expect(out.buffers).toEqual([])
    })
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

    const out = validateLoadedBuffers(strandedOnly, ROOT)
    expect(out.panes.root.editorTabIds).toEqual(['e-1'])
    expect(out.panes.root.activeEditorTabId).toBe('e-1')
  })

  it('drops a buffer no pane lists — a leak from before C2 held is not restored', () => {
    const leaked = {
      buffers: [
        { id: 'e-1', type: 'editor', path: '/a.ts', name: 'a.ts' },
        { id: 't-1', type: 'terminal', sessionId: 's', name: 'Terminal 1' },
      ],
      panes: {
        root: { id: 'root', editorTabIds: ['e-1'], activeEditorTabId: 'e-1' },
      },
    } as unknown as Snapshot

    expect(validateLoadedBuffers(leaked, ROOT).buffers.map((b) => b.id)).toEqual(['e-1'])
  })

  it('re-homes an unsaved buffer no pane lists instead of dropping it', () => {
    const leaked = {
      buffers: [
        { id: 'e-1', type: 'editor', path: '/a.ts', name: 'a.ts' },
        { id: 'e-2', type: 'editor', path: '/b.ts', content: 'edit', savedContent: 'disk' },
        {
          id: 'e-3',
          type: 'editor',
          path: '/c.ts',
          content: 'x',
          savedContent: 'x',
          isDirty: true,
        },
      ],
      panes: {
        root: { id: 'root', editorTabIds: [], activeEditorTabId: null },
        other: { id: 'other', editorTabIds: ['e-1'], activeEditorTabId: 'e-1' },
      },
    } as unknown as Snapshot

    const out = validateLoadedBuffers(leaked, ROOT)

    expect(out.buffers.map((b) => b.id)).toEqual(['e-1', 'e-2', 'e-3'])
    expect(out.panes.root.editorTabIds).toEqual(['e-2', 'e-3'])
    expect(out.panes.root.activeEditorTabId).toBe('e-2')
  })

  it('keeps every unlisted buffer when asked to (a layout saved without views)', () => {
    const unlisted = {
      buffers: [{ id: 'e-1', type: 'editor', path: '/a.ts', name: 'a.ts' }],
      panes: { root: { id: 'root', editorTabIds: [], activeEditorTabId: null } },
    } as unknown as Snapshot

    const out = validateLoadedBuffers(unlisted, { adoptInto: 'root', keepUnlisted: true })

    expect(out.buffers.map((b) => b.id)).toEqual(['e-1'])
    expect(out.panes.root.editorTabIds).toEqual(['e-1'])
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

    const out = validateLoadedBuffers(stale, ROOT)

    expect(out.buffers.map((b) => b.id)).toEqual(['e-1'])
    // The pane must not be left pointing at the id that just vanished.
    expect(out.panes.root.editorTabIds).toEqual(['e-1'])
    expect(out.panes.root.activeEditorTabId).toBe('e-1')
  })

  it('keeps every type this build DOES know, including the new commit diff', () => {
    const current = {
      buffers: [
        { id: 'c-1', type: 'commitDiff', wsId: 'w1', sha: 'abc1234', name: 'Commit abc1234' },
        {
          id: 'br-1',
          type: 'branchReview',
          wsId: 'w1',
          name: 'Branch Review',
          view: 'conversation',
        },
      ],
      panes: {
        root: { id: 'root', editorTabIds: ['c-1', 'br-1'], activeEditorTabId: 'c-1' },
      },
    } as unknown as Snapshot

    expect(validateLoadedBuffers(current, ROOT)).toEqual(current)
  })

  // Distinct from the retired `newTab` placeholder above, which old layouts
  // still carry and must keep dropping.
  it('restores a New Tab page where it was, beside the layouts that predate it', () => {
    const live = {
      buffers: [
        {
          id: 'p-1',
          type: 'newTabPage',
          path: 'new-tab://p-1',
          name: 'New Tab',
          workspaceId: 'w1',
        },
        { id: 'nt-1', type: 'newTab', path: '', name: 'New Tab' },
        { id: 'e-1', type: 'editor', path: '/a.ts', name: 'a.ts' },
      ],
      panes: {
        root: { id: 'root', editorTabIds: ['e-1', 'p-1', 'nt-1'], activeEditorTabId: 'p-1' },
      },
    } as unknown as Snapshot

    const out = validateLoadedBuffers({ ...live, buffers: toPersistedBuffers(live.buffers) }, ROOT)

    expect(out.buffers.map((b) => b.id)).toEqual(['p-1', 'e-1'])
    expect(out.buffers[0]).toEqual(live.buffers[0])
    expect(out.panes.root.editorTabIds).toEqual(['e-1', 'p-1'])
    expect(out.panes.root.activeEditorTabId).toBe('p-1')
  })
})
