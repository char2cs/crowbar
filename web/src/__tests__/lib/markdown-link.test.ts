import { beforeEach, describe, expect, it, vi } from 'vitest'

// The transport (`openExternalUrl`) is exercised for real elsewhere; here we
// only care that the click handler ROUTES to it, so mock it to a spy.
vi.mock('@/lib/external-open', () => ({
  openExternalUrl: vi.fn(),
}))

import { openExternalUrl } from '@/lib/external-open'
import { handleMarkdownAnchorClick, isExternalUrl } from '@/lib/markdown-link'

function fakeClick() {
  return { preventDefault: vi.fn(), stopPropagation: vi.fn() }
}

describe('isExternalUrl', () => {
  it.each([
    ['https://example.com', true],
    ['http://example.com', true],
    ['HTTPS://EXAMPLE.COM', true],
    ['mailto:someone@example.com', true],
    ['tel:+15551234567', true],
    ['//cdn.example.com/logo.png', true],
    ['./relative/spec.md', false],
    ['../up-one/readme.md', false],
    ['/abs/path.md', false],
    ['#heading-anchor', false],
    ['', false],
  ])('classifies %s as external=%s', (url, expected) => {
    expect(isExternalUrl(url)).toBe(expected)
  })
})

describe('handleMarkdownAnchorClick', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('opens an external URL in the OS browser and cancels the in-app navigation', () => {
    const e = fakeClick()
    handleMarkdownAnchorClick(e, 'https://example.com/docs')

    // In the Tauri WKWebView a bare <a> click otherwise replaces the whole app.
    expect(e.preventDefault).toHaveBeenCalledOnce()
    expect(e.stopPropagation).toHaveBeenCalledOnce()
    expect(openExternalUrl).toHaveBeenCalledWith('https://example.com/docs')
  })

  it('still cancels the webview navigation for a non-external link but does not shell-open it', () => {
    const e = fakeClick()
    handleMarkdownAnchorClick(e, './notes/spec.md')

    expect(e.preventDefault).toHaveBeenCalledOnce()
    expect(openExternalUrl).not.toHaveBeenCalled()
  })

  it('does nothing when there is no href', () => {
    const e = fakeClick()
    handleMarkdownAnchorClick(e, null)

    expect(e.preventDefault).not.toHaveBeenCalled()
    expect(openExternalUrl).not.toHaveBeenCalled()
  })
})
