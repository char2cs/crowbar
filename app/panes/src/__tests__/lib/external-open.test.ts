import { afterEach, describe, expect, it, vi } from 'vitest'

const { shellOpen } = vi.hoisted(() => ({ shellOpen: vi.fn(async (_url: string) => {}) }))
vi.mock('@tauri-apps/plugin-shell', () => ({ open: shellOpen }))
const platform = vi.hoisted(() => ({ mobile: false }))
vi.mock('@/utils/platform', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/utils/platform')>()),
  get IS_MOBILE() {
    return platform.mobile
  },
}))

import { openExternalUrl } from '@/lib/external-open'

afterEach(() => {
  platform.mobile = false
  vi.restoreAllMocks()
  shellOpen.mockClear()
  delete (window as unknown as Record<string, unknown>).__TAURI_INTERNALS__
})

const OPENABLE = [
  'https://example.com/a',
  'http://example.com',
  'mailto:me@example.com',
  'tel:+15551234567',
]

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

describe('openExternalUrl in the desktop app', () => {
  it('hands a web link to the OS opener and refuses anything else before it', async () => {
    ;(window as unknown as Record<string, unknown>).__TAURI_INTERNALS__ = {}
    await openExternalUrl('https://example.com/a')
    await expect(openExternalUrl('file:///etc/passwd')).rejects.toThrow()
    expect(shellOpen.mock.calls).toEqual([['https://example.com/a']])
  })
})

describe('openExternalUrl in the phone app', () => {
  it('refuses, opening nothing in or over the app', async () => {
    platform.mobile = true
    ;(window as unknown as Record<string, unknown>).__TAURI_INTERNALS__ = { invoke: vi.fn() }
    const open = vi.spyOn(window, 'open').mockReturnValue(null)

    await expect(openExternalUrl('https://example.com/a')).rejects.toThrow(/phone app/)

    expect(shellOpen).not.toHaveBeenCalled()
    expect(open).not.toHaveBeenCalled()
  })

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
    expect(shellOpen).not.toHaveBeenCalled()
  })
})
