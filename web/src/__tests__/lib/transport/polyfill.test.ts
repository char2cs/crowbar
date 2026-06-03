import '@/lib/transport/polyfill'
import { describe, it, expect, vi } from 'vitest'

describe('window.__CROWBAR__ polyfill', () => {
  it('sets mode to local', () => {
    expect(window.__CROWBAR__.mode).toBe('local')
  })

  it('sets endpoint from env or default', () => {
    expect(typeof window.__CROWBAR__.endpoint).toBe('string')
    expect(window.__CROWBAR__.endpoint.length).toBeGreaterThan(0)
  })

  it('on() returns an unlisten function', () => {
    const unlisten = window.__CROWBAR__.on('test:event', () => {})
    expect(typeof unlisten).toBe('function')
    unlisten()
  })

  it('emit() calls registered handlers', () => {
    const handler = vi.fn()
    const unlisten = window.__CROWBAR__.on('test:ping', handler)
    window.__CROWBAR__.emit('test:ping', { value: 42 })
    expect(handler).toHaveBeenCalledWith({ value: 42 })
    unlisten()
  })

  it('unlisten removes the handler', () => {
    const handler = vi.fn()
    const unlisten = window.__CROWBAR__.on('test:remove', handler)
    unlisten()
    window.__CROWBAR__.emit('test:remove', {})
    expect(handler).not.toHaveBeenCalled()
  })
})
