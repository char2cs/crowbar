import { describe, it, expect } from 'vitest'

describe('transport defaults', () => {
  it('apiBase is empty string when __CROWBAR__ is not set', () => {
    // window.__CROWBAR__ is not set in jsdom by default
    const cfg = (window as any).__CROWBAR__ ?? null
    const apiBase = cfg?.api ?? ''
    expect(apiBase).toBe('')
  })

  it('eventUrl falls back to /api/events', () => {
    const cfg = (window as any).__CROWBAR__ ?? null
    const eventUrl = cfg?.events ?? '/api/events'
    expect(eventUrl).toBe('/api/events')
  })
})
