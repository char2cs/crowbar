import { afterEach, describe, expect, it, vi } from 'vitest'
import { postShellFrame } from '@/sdk/post-shell-frame'
import type { PaneFrame } from '@/sdk/pane-frame'

afterEach(() => {
  delete (window as unknown as Record<string, unknown>).crowbar
})

describe('postShellFrame', () => {
  it.each<[string, PaneFrame]>([
    ['openExternal', { kind: 'shell.openExternal', url: 'https://example.com/' }],
    ['reveal', { kind: 'shell.reveal', path: '/tmp/a' }],
    ['command', { kind: 'command.run', command: 'app.openLog' }],
  ])('posts a %s frame inside the shell', (_name, frame) => {
    const post = vi.fn()
    ;(window as unknown as Record<string, unknown>).crowbar = { post, onmessage: null }
    expect(postShellFrame(frame)).toBe(true)
    expect(JSON.parse(post.mock.calls[0][0])).toEqual({ sdk: 1, ...frame })
  })

  it('does nothing outside the shell', () => {
    expect(postShellFrame({ kind: 'shell.reveal', path: '/tmp/a' })).toBe(false)
  })
})
