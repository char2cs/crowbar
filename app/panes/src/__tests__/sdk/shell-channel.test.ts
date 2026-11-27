import { afterEach, describe, expect, it, vi } from 'vitest'
import { shellChannel } from '@/sdk/shell-channel'

afterEach(() => {
  delete (window as unknown as Record<string, unknown>).crowbar
})

describe('shellChannel', () => {
  it.each([
    ['is null outside the shell', undefined, false],
    ['is null when the injected object cannot post', {}, false],
    ['is the injected channel inside the shell', { post: vi.fn(), onmessage: null }, true],
  ])('%s', (_name, injected, expected) => {
    ;(window as unknown as Record<string, unknown>).crowbar = injected
    expect(shellChannel() !== null).toBe(expected)
  })
})
