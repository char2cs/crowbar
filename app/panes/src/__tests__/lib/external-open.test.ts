import { afterEach, describe, expect, it, vi } from 'vitest'

import { openExternalUrl } from '@/lib/external-open'

afterEach(() => {
  vi.restoreAllMocks()
  delete (window as unknown as Record<string, unknown>).__TAURI_INTERNALS__
})

const OPENABLE = [
  'https://example.com/a',
  'http://example.com',
  'mailto:me@example.com',
  'tel:+15551234567',
]

// A link's text can come from another host: only a web, mail or phone link leaves the app.
const REFUSED = [
  'javascript:alert(1)',
  ' javascript:alert(1)',
  'JaVaScRiPt:alert(1)',
  'data:text/html,<script>alert(1)</script>',
  'file:///etc/passwd',
  'vscode://file/etc/passwd',
  'crowbar://local/v0/projects',
  '//evil.example/x',
  'https:',
  '',
]

describe('openExternalUrl in the browser', () => {
  it.each(OPENABLE)('opens %s in a new tab with no opener', async (url) => {
    const open = vi.spyOn(window, 'open').mockReturnValue(null)
    await openExternalUrl(url)
    expect(open).toHaveBeenCalledExactlyOnceWith(url, '_blank', 'noopener,noreferrer')
  })

  it.each(REFUSED)('refuses %j and opens nothing', async (url) => {
    const open = vi.spyOn(window, 'open').mockReturnValue(null)
    await expect(openExternalUrl(url)).rejects.toThrow(/not a web, mail or phone link/)
    expect(open).not.toHaveBeenCalled()
  })
})

// The phone app has no opener, and a link opened in its one webview would replace the app.
describe('openExternalUrl in the phone app', () => {
  it('refuses, opening nothing in or over the app', async () => {
    ;(window as unknown as Record<string, unknown>).__TAURI_INTERNALS__ = { invoke: vi.fn() }
    const open = vi.spyOn(window, 'open').mockReturnValue(null)

    await expect(openExternalUrl('https://example.com/a')).rejects.toThrow(/phone app/)

    expect(open).not.toHaveBeenCalled()
  })
})

describe('openExternalUrl in the shell', () => {
  it('posts a shell.openExternal frame inside the shell instead of opening it itself', async () => {
    const post = vi.fn()
    ;(window as unknown as Record<string, unknown>).crowbar = { post, onmessage: null }
    await openExternalUrl('https://example.com/a')
    delete (window as unknown as Record<string, unknown>).crowbar
    expect(JSON.parse(post.mock.calls[0][0])).toEqual({
      sdk: 1,
      kind: 'shell.openExternal',
      url: 'https://example.com/a',
    })
  })
})
