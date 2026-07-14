import { describe, expect, it, vi } from 'vitest'
import { createActiveEditorRegistry } from '@/features/editor/lib/active-editor-context'

const ctx = (uri: string, paneId = 'p1') => ({ paneId, uri, filePath: uri })

describe('createActiveEditorRegistry', () => {
  it('set then get returns the latest context for a pane', () => {
    const r = createActiveEditorRegistry()
    r.set('p1', ctx('crowbar://editor/a'))
    expect(r.get('p1')?.uri).toBe('crowbar://editor/a')
  })

  it('subscribe fires immediately with current context, then on every change', () => {
    const r = createActiveEditorRegistry()
    r.set('p1', ctx('crowbar://editor/a'))
    const cb = vi.fn()
    r.subscribe('p1', cb)
    expect(cb).toHaveBeenCalledTimes(1) // immediate with current
    expect(cb).toHaveBeenLastCalledWith(expect.objectContaining({ uri: 'crowbar://editor/a' }))
    r.set('p1', ctx('crowbar://editor/b'))
    expect(cb).toHaveBeenCalledTimes(2)
    expect(cb).toHaveBeenLastCalledWith(expect.objectContaining({ uri: 'crowbar://editor/b' }))
  })

  it('does NOT refire when set with the same uri (de-dupe)', () => {
    const r = createActiveEditorRegistry()
    r.set('p1', ctx('crowbar://editor/a'))
    const cb = vi.fn()
    r.subscribe('p1', cb) // 1 (immediate)
    r.set('p1', ctx('crowbar://editor/a')) // same uri -> no refire
    expect(cb).toHaveBeenCalledTimes(1)
  })

  it('isolates panes: a change to p2 does not call p1 subscribers', () => {
    const r = createActiveEditorRegistry()
    const cb1 = vi.fn()
    r.subscribe('p1', cb1) // 1 (immediate, undefined)
    r.set('p2', ctx('crowbar://editor/x', 'p2'))
    expect(cb1).toHaveBeenCalledTimes(1)
  })

  it('unsubscribe stops further calls', () => {
    const r = createActiveEditorRegistry()
    const cb = vi.fn()
    const off = r.subscribe('p1', cb) // 1 (immediate undefined)
    off()
    r.set('p1', ctx('crowbar://editor/a'))
    expect(cb).toHaveBeenCalledTimes(1)
  })

  it('clear(paneId) notifies subscribers with undefined and drops state', () => {
    const r = createActiveEditorRegistry()
    r.set('p1', ctx('crowbar://editor/a'))
    const cb = vi.fn()
    r.subscribe('p1', cb) // 1 immediate
    r.clear('p1')
    expect(cb).toHaveBeenLastCalledWith(undefined)
    expect(r.get('p1')).toBeUndefined()
  })

  // R12: a close→reopen of the same file reuses the uri but recreates the model
  // (the old one was disposed). Deduping on uri alone retained the disposed model
  // and never re-notified satellites, crashing on the next read. The dedup must
  // therefore compare model identity too.
  it('RE-notifies when set with a NEW model for the SAME uri (model-identity dedup)', () => {
    const r = createActiveEditorRegistry()
    const m1 = { id: 'model-1' }
    const m2 = { id: 'model-2' }
    r.set('p1', { ...ctx('crowbar://editor/a'), model: m1 })
    const cb = vi.fn()
    r.subscribe('p1', cb) // 1 immediate (m1)

    // Same uri but a FRESH model (close then reopen) → must re-notify.
    r.set('p1', { ...ctx('crowbar://editor/a'), model: m2 })
    expect(cb).toHaveBeenCalledTimes(2)
    expect(cb).toHaveBeenLastCalledWith(expect.objectContaining({ model: m2 }))
    expect(r.get('p1')?.model).toBe(m2)
  })

  it('still de-dupes when set with the SAME uri AND the SAME model', () => {
    const r = createActiveEditorRegistry()
    const model = { id: 'model-1' }
    r.set('p1', { ...ctx('crowbar://editor/a'), model })
    const cb = vi.fn()
    r.subscribe('p1', cb) // 1 immediate
    r.set('p1', { ...ctx('crowbar://editor/a'), model }) // identical → no refire
    expect(cb).toHaveBeenCalledTimes(1)
  })

  it('clearIfActive(paneId, uri) clears only when the current uri matches', () => {
    const r = createActiveEditorRegistry()
    r.set('p1', ctx('crowbar://editor/a'))
    const cb = vi.fn()
    r.subscribe('p1', cb) // 1 immediate

    // Different uri → no-op (the pane already moved on).
    r.clearIfActive('p1', 'crowbar://editor/other')
    expect(cb).toHaveBeenCalledTimes(1)
    expect(r.get('p1')?.uri).toBe('crowbar://editor/a')

    // Matching uri → clears and notifies with undefined.
    r.clearIfActive('p1', 'crowbar://editor/a')
    expect(cb).toHaveBeenCalledTimes(2)
    expect(cb).toHaveBeenLastCalledWith(undefined)
    expect(r.get('p1')).toBeUndefined()
  })

  it('clearIfActive is a no-op for an unknown pane', () => {
    const r = createActiveEditorRegistry()
    expect(() => r.clearIfActive('nope', 'crowbar://editor/a')).not.toThrow()
  })
})
