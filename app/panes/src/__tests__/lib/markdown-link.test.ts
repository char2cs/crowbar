import { beforeEach, describe, expect, it, vi } from 'vitest'

// The transport (`openExternalUrl`) is exercised for real elsewhere; here we
// only care that the click handler ROUTES to it, so mock it to a spy.
vi.mock('@/lib/external-open', () => ({
  openExternalUrl: vi.fn(async () => {}),
}))

import { openExternalUrl } from '@/lib/external-open'
import { handleMarkdownAnchorClick, isExternalUrl } from '@/lib/markdown-link'
import { toast } from '@/features/window/stores/toast-store'
import { __resetWorkspaceScopesForTest, recordWorkspaceScope } from '@/lib/workspace-scope'
import {
  loadProjects,
  pairRemoteHost,
  projectOn,
  resetHostWorld,
} from '@/__tests__/__fixtures__/remote-host'

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
    handleMarkdownAnchorClick(e, 'https://example.com/docs', null)

    // In the Tauri WKWebView a bare <a> click otherwise replaces the whole app.
    expect(e.preventDefault).toHaveBeenCalledOnce()
    expect(e.stopPropagation).toHaveBeenCalledOnce()
    expect(openExternalUrl).toHaveBeenCalledWith('https://example.com/docs')
  })

  it('opens a protocol-relative link as https', () => {
    handleMarkdownAnchorClick(fakeClick(), '//cdn.example.com/logo.png', null)

    expect(openExternalUrl).toHaveBeenCalledExactlyOnceWith('https://cdn.example.com/logo.png')
  })

  it('still cancels the webview navigation for a non-external link but does not shell-open it', () => {
    const e = fakeClick()
    handleMarkdownAnchorClick(e, './notes/spec.md', null)

    expect(e.preventDefault).toHaveBeenCalledOnce()
    expect(openExternalUrl).not.toHaveBeenCalled()
  })

  it('does nothing when there is no href', () => {
    const e = fakeClick()
    handleMarkdownAnchorClick(e, null, null)

    expect(e.preventDefault).not.toHaveBeenCalled()
    expect(openExternalUrl).not.toHaveBeenCalled()
  })

  // A chat or README in a workspace on another host that links its own localhost means that host.
  it('does not open a loopback link from a workspace on another host', () => {
    resetHostWorld()
    __resetWorkspaceScopesForTest()
    const info = vi.spyOn(toast, 'info').mockReturnValue('')
    pairRemoteHost('h1', 'Build box')
    loadProjects(projectOn('p', 'h1'))
    recordWorkspaceScope({ projectId: 'p', repoId: 'r', wsId: 'w-remote' })
    const e = fakeClick()

    handleMarkdownAnchorClick(e, 'http://localhost:3000/', 'w-remote')

    expect(e.preventDefault).toHaveBeenCalledOnce()
    expect(openExternalUrl).not.toHaveBeenCalled()
    expect(info.mock.calls[0][0]).toMatch(/^This link is on Build box, not this computer/)
    info.mockRestore()
    resetHostWorld()
  })
})
