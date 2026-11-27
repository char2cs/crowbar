import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { ActiveEditorRegistry } from '@/features/editor/lib/active-editor-context'

const ctx = (uri: string, paneId = 'p1') => ({ paneId, uri, filePath: uri })

let freshRegistry: ActiveEditorRegistry
beforeEach(async () => {
  vi.resetModules()
  freshRegistry = (await import('@/features/editor/lib/active-editor-context')).activeEditors
})

describe('activeEditors', () => {
  it('set then get returns the latest context for a pane', () => {
    const r = freshRegistry
    r.set('p1', ctx('crowbar://editor/a'))
    expect(r.get('p1')?.uri).toBe('crowbar://editor/a')
  })

  it('subscribe fires immediately with current context, then on every change', () => {
    const r = freshRegistry
    r.set('p1', ctx('crowbar://editor/a'))
    const cb = vi.fn()
    r.subscribe('p1', cb)
    expect(cb).toHaveBeenCalledTimes(1)
    expect(cb).toHaveBeenLastCalledWith(expect.objectContaining({ uri: 'crowbar://editor/a' }))
    r.set('p1', ctx('crowbar://editor/b'))
    expect(cb).toHaveBeenCalledTimes(2)
    expect(cb).toHaveBeenLastCalledWith(expect.objectContaining({ uri: 'crowbar://editor/b' }))
  })

  it('does NOT refire when set with the same uri (de-dupe)', () => {
    const r = freshRegistry
    r.set('p1', ctx('crowbar://editor/a'))
    const cb = vi.fn()
    r.subscribe('p1', cb)
    r.set('p1', ctx('crowbar://editor/a'))
    expect(cb).toHaveBeenCalledTimes(1)
  })

  it('isolates panes: a change to p2 does not call p1 subscribers', () => {
    const r = freshRegistry
    const cb1 = vi.fn()
    r.subscribe('p1', cb1)
    r.set('p2', ctx('crowbar://editor/x', 'p2'))
    expect(cb1).toHaveBeenCalledTimes(1)
  })

  it('unsubscribe stops further calls', () => {
    const r = freshRegistry
    const cb = vi.fn()
    const off = r.subscribe('p1', cb)
    off()
    r.set('p1', ctx('crowbar://editor/a'))
    expect(cb).toHaveBeenCalledTimes(1)
  })

  it('clear(paneId) notifies subscribers with undefined and drops state', () => {
    const r = freshRegistry
    r.set('p1', ctx('crowbar://editor/a'))
    const cb = vi.fn()
    r.subscribe('p1', cb)
    r.clear('p1')
    expect(cb).toHaveBeenLastCalledWith(undefined)
    expect(r.get('p1')).toBeUndefined()
  })

  it('RE-notifies when set with a NEW model for the SAME uri (model-identity dedup)', () => {
    const r = freshRegistry
    const m1 = { id: 'model-1' }
    const m2 = { id: 'model-2' }
    r.set('p1', { ...ctx('crowbar://editor/a'), model: m1 })
    const cb = vi.fn()
    r.subscribe('p1', cb)

    r.set('p1', { ...ctx('crowbar://editor/a'), model: m2 })
    expect(cb).toHaveBeenCalledTimes(2)
    expect(cb).toHaveBeenLastCalledWith(expect.objectContaining({ model: m2 }))
    expect(r.get('p1')?.model).toBe(m2)
  })

  it('still de-dupes when set with the SAME uri AND the SAME model', () => {
    const r = freshRegistry
    const model = { id: 'model-1' }
    r.set('p1', { ...ctx('crowbar://editor/a'), model })
    const cb = vi.fn()
    r.subscribe('p1', cb)
    r.set('p1', { ...ctx('crowbar://editor/a'), model })
    expect(cb).toHaveBeenCalledTimes(1)
  })

  it('clearIfActive(paneId, uri) clears only when the current uri matches', () => {
    const r = freshRegistry
    r.set('p1', ctx('crowbar://editor/a'))
    const cb = vi.fn()
    r.subscribe('p1', cb)

    r.clearIfActive('p1', 'crowbar://editor/other')
    expect(cb).toHaveBeenCalledTimes(1)
    expect(r.get('p1')?.uri).toBe('crowbar://editor/a')

    r.clearIfActive('p1', 'crowbar://editor/a')
    expect(cb).toHaveBeenCalledTimes(2)
    expect(cb).toHaveBeenLastCalledWith(undefined)
    expect(r.get('p1')).toBeUndefined()
  })

  it('clearIfActive is a no-op for an unknown pane', () => {
    const r = freshRegistry
    expect(() => r.clearIfActive('nope', 'crowbar://editor/a')).not.toThrow()
  })
})
