import { describe, expect, it, vi } from 'vitest'
import { createActiveEditorRegistry } from '@/features/editor/lib/active-editor-context'

const ctx = (uri: string, paneId = 'p1') => ({ paneId, uri, filePath: uri })

describe('createActiveEditorRegistry', () => {
  it('set then get returns the latest context for a pane', () => {
    const r = createActiveEditorRegistry()
    r.set('p1', ctx('athas://editor/a'))
    expect(r.get('p1')?.uri).toBe('athas://editor/a')
  })

  it('subscribe fires immediately with current context, then on every change', () => {
    const r = createActiveEditorRegistry()
    r.set('p1', ctx('athas://editor/a'))
    const cb = vi.fn()
    r.subscribe('p1', cb)
    expect(cb).toHaveBeenCalledTimes(1)                 // immediate with current
    expect(cb).toHaveBeenLastCalledWith(expect.objectContaining({ uri: 'athas://editor/a' }))
    r.set('p1', ctx('athas://editor/b'))
    expect(cb).toHaveBeenCalledTimes(2)
    expect(cb).toHaveBeenLastCalledWith(expect.objectContaining({ uri: 'athas://editor/b' }))
  })

  it('does NOT refire when set with the same uri (de-dupe)', () => {
    const r = createActiveEditorRegistry()
    r.set('p1', ctx('athas://editor/a'))
    const cb = vi.fn()
    r.subscribe('p1', cb)            // 1 (immediate)
    r.set('p1', ctx('athas://editor/a'))   // same uri -> no refire
    expect(cb).toHaveBeenCalledTimes(1)
  })

  it('isolates panes: a change to p2 does not call p1 subscribers', () => {
    const r = createActiveEditorRegistry()
    const cb1 = vi.fn()
    r.subscribe('p1', cb1)          // 1 (immediate, undefined)
    r.set('p2', ctx('athas://editor/x', 'p2'))
    expect(cb1).toHaveBeenCalledTimes(1)
  })

  it('unsubscribe stops further calls', () => {
    const r = createActiveEditorRegistry()
    const cb = vi.fn()
    const off = r.subscribe('p1', cb)   // 1 (immediate undefined)
    off()
    r.set('p1', ctx('athas://editor/a'))
    expect(cb).toHaveBeenCalledTimes(1)
  })

  it('clear(paneId) notifies subscribers with undefined and drops state', () => {
    const r = createActiveEditorRegistry()
    r.set('p1', ctx('athas://editor/a'))
    const cb = vi.fn()
    r.subscribe('p1', cb)   // 1 immediate
    r.clear('p1')
    expect(cb).toHaveBeenLastCalledWith(undefined)
    expect(r.get('p1')).toBeUndefined()
  })
})
