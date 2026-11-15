import { afterEach, describe, expect, it, vi } from 'vitest'
import { inShell } from '@/lib/ws/in-shell'

const win = window as unknown as Record<string, unknown>

afterEach(() => {
  delete win.crowbar
  delete win.__CROWBAR__
})

describe('inShell', () => {
  it('is false outside the shell and true once the shell injected its channel and endpoints', () => {
    expect(inShell()).toBe(false)
    win.crowbar = { post: vi.fn(), onmessage: null }
    win.__CROWBAR__ = { api: 'crowbar://local', host: 'local' }
    expect(inShell()).toBe(true)
  })
})
