import { afterEach, beforeEach, describe, it, expect, vi } from 'vitest'

// The polyfill runs once at import and reads VITE_API_URL, so each test owns the
// env it imports under: the result must not depend on the shell that ran vitest.
async function loadPolyfill(apiUrl: string | undefined) {
  vi.stubEnv('VITE_API_URL', apiUrl)
  vi.resetModules()
  Reflect.deleteProperty(window, '__CROWBAR__')
  await import('@/lib/transport/polyfill')
}

describe('window.__CROWBAR__ polyfill', () => {
  beforeEach(async () => {
    await loadPolyfill(undefined)
  })

  afterEach(() => {
    vi.unstubAllEnvs()
  })

  it('sets mode to local', () => {
    expect(window.__CROWBAR__.mode).toBe('local')
  })

  it('sets endpoint from env', async () => {
    await loadPolyfill('http://daemon.test:9000')
    expect(window.__CROWBAR__.endpoint).toBe('http://daemon.test:9000')
  })

  it('falls back to the default endpoint when env is unset', () => {
    expect(window.__CROWBAR__.endpoint).toBe('http://localhost:7457')
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
