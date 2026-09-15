import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  clearEditorPortalEntry,
  getEditorPortalEntry,
  setEditorPortalEntry,
  subscribeEditorPortalEntry,
  type EditorPortalEntry,
} from '@/features/panes/lib/editor-portal-registry'

// Module-level singleton (one registry for the whole window, matching
// windowPaneStore itself — see the module's own doc) — each test uses its
// own paneId so entries never collide, but we still clear defensively.
afterEach(() => {
  clearEditorPortalEntry('p1')
  clearEditorPortalEntry('p2')
})

function entry(overrides: Partial<EditorPortalEntry> = {}): EditorPortalEntry {
  return {
    node: document.createElement('div'),
    activeEditorBufferId: 'buf-a',
    isPreview: false,
    isActiveSurface: true,
    ...overrides,
  }
}

describe('editor-portal-registry', () => {
  it('set then get returns the latest entry for a pane', () => {
    const e = entry()
    setEditorPortalEntry('p1', e)
    expect(getEditorPortalEntry('p1')).toBe(e)
  })

  it('subscribe fires immediately with the current entry, then on every change', () => {
    const e1 = entry()
    setEditorPortalEntry('p1', e1)
    const cb = vi.fn()
    subscribeEditorPortalEntry('p1', cb)
    expect(cb).toHaveBeenCalledTimes(1)
    expect(cb).toHaveBeenLastCalledWith(e1)

    const e2 = entry({ activeEditorBufferId: 'buf-b' })
    setEditorPortalEntry('p1', e2)
    expect(cb).toHaveBeenCalledTimes(2)
    expect(cb).toHaveBeenLastCalledWith(e2)
  })

  it('subscribe fires immediately with undefined when nothing is published yet', () => {
    const cb = vi.fn()
    subscribeEditorPortalEntry('p1', cb)
    expect(cb).toHaveBeenCalledTimes(1)
    expect(cb).toHaveBeenLastCalledWith(undefined)
  })

  it('isolates panes: publishing p2 does not notify p1 subscribers', () => {
    const cb1 = vi.fn()
    subscribeEditorPortalEntry('p1', cb1) // 1: immediate, undefined
    setEditorPortalEntry('p2', entry())
    expect(cb1).toHaveBeenCalledTimes(1)
  })

  it('unsubscribe stops further calls', () => {
    const cb = vi.fn()
    const off = subscribeEditorPortalEntry('p1', cb) // 1: immediate
    off()
    setEditorPortalEntry('p1', entry())
    expect(cb).toHaveBeenCalledTimes(1)
  })

  it('clearEditorPortalEntry notifies subscribers with undefined and drops the entry', () => {
    setEditorPortalEntry('p1', entry())
    const cb = vi.fn()
    subscribeEditorPortalEntry('p1', cb) // 1: immediate
    clearEditorPortalEntry('p1')
    expect(cb).toHaveBeenLastCalledWith(undefined)
    expect(getEditorPortalEntry('p1')).toBeUndefined()
  })

  it('clearing a pane with no entry and no subscribers is a harmless no-op', () => {
    expect(() => clearEditorPortalEntry('p1')).not.toThrow()
  })
})
