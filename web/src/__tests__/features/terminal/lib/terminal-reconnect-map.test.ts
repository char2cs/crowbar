import { describe, it, expect, beforeEach } from 'vitest'
import {
  saveReconnect,
  loadReconnect,
  clearReconnect,
} from '@/features/terminal/lib/terminal-reconnect-map'

beforeEach(() => localStorage.clear())

describe('terminal-reconnect-map', () => {
  it('round-trips a mapping', () => {
    saveReconnect('ws-1', 'tab-1', 'conn-1')
    expect(loadReconnect('ws-1', 'tab-1')).toBe('conn-1')
  })
  it('isolates by workspace and tab', () => {
    saveReconnect('ws-1', 'tab-1', 'conn-1')
    expect(loadReconnect('ws-2', 'tab-1')).toBeNull()
    expect(loadReconnect('ws-1', 'tab-2')).toBeNull()
  })
  it('clears a mapping', () => {
    saveReconnect('ws-1', 'tab-1', 'conn-1')
    clearReconnect('ws-1', 'tab-1')
    expect(loadReconnect('ws-1', 'tab-1')).toBeNull()
  })
})
