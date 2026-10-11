import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('@/lib/external-open', () => ({ openExternalUrl: vi.fn(async () => {}) }))

import { toast } from '@/features/window/stores/toast-store'
import { openExternalUrl } from '@/lib/external-open'
import { openWorkspaceLink } from '@/lib/workspace-link'
import { __resetWorkspaceScopesForTest, recordWorkspaceScope } from '@/lib/workspace-scope'
import {
  loadProjects,
  pairRemoteHost,
  projectOn,
  resetHostWorld,
} from '@/__tests__/__fixtures__/remote-host'

let info: ReturnType<typeof vi.spyOn>

beforeEach(() => {
  vi.mocked(openExternalUrl).mockClear()
  resetHostWorld()
  __resetWorkspaceScopesForTest()
  info = vi.spyOn(toast, 'info').mockReturnValue('')
  pairRemoteHost('h1', 'Build box')
  loadProjects(projectOn('p-remote', 'h1'), projectOn('p-local', 'local'))
  recordWorkspaceScope({ projectId: 'p-remote', repoId: 'r', wsId: 'w-remote' })
  recordWorkspaceScope({ projectId: 'p-local', repoId: 'r', wsId: 'w-local' })
})

afterEach(() => {
  info.mockRestore()
  resetHostWorld()
})

// Every spelling a URL parser turns into this computer's own address.
const LOOPBACK = [
  'http://localhost:3000/',
  'http://LOCALHOST.:3000/',
  'http://app.localhost/',
  'http://127.0.0.1:3000/',
  'http://127.1:3000/',
  'http://2130706433:3000/',
  'http://0x7f.1:3000/',
  'http://0:3000/',
  'http://0.0.0.0:3000/',
  'http://[::1]:3000/',
  'http://[0:0:0:0:0:0:0:1]:3000/',
  'http://[::]:3000/',
  'http://[::ffff:127.0.0.1]:3000/',
  'http://[::ffff:7f00:1]:3000/',
  'http://[::ffff:127.255.0.9]/',
  'http://evil.com@localhost:3000/',
  '//localhost:3000/',
]

describe('a link from a workspace on another host', () => {
  it.each(LOOPBACK)('does not open %s here: it is that host’s', async (url) => {
    await openWorkspaceLink(url, 'w-remote')

    expect(openExternalUrl).not.toHaveBeenCalled()
    expect(info.mock.calls[0]?.[0]).toMatch(/^This link is on Build box, not this computer/)
  })

  it.each([
    ['http://localhost:3000@evil.com/', 'http://localhost:3000@evil.com/'],
    ['https://example.com/docs', 'https://example.com/docs'],
    ['http://[::ffff:8.8.8.8]/', 'http://[::ffff:8.8.8.8]/'],
    ['//cdn.example.com/logo.png', 'https://cdn.example.com/logo.png'],
  ])('opens %s, which is not a loopback address', async (url, opened) => {
    await openWorkspaceLink(url, 'w-remote')

    expect(openExternalUrl).toHaveBeenCalledExactlyOnceWith(opened)
    expect(info).not.toHaveBeenCalled()
  })
})

describe('a link from this computer’s own content', () => {
  it.each([
    ['a local workspace', 'w-local'],
    ['no workspace', null],
  ])('in %s opens a loopback address here', async (_, wsId) => {
    await openWorkspaceLink('http://localhost:3000/', wsId)

    expect(openExternalUrl).toHaveBeenCalledExactlyOnceWith('http://localhost:3000/')
  })

  // Nothing in the app has a page scheme for `//host` to inherit: it is a web link.
  it('opens a protocol-relative link as https', async () => {
    await openWorkspaceLink('//cdn.example.com/logo.png', 'w-local')
    await openWorkspaceLink('//localhost:3000/', null)

    expect(vi.mocked(openExternalUrl).mock.calls).toEqual([
      ['https://cdn.example.com/logo.png'],
      ['https://localhost:3000/'],
    ])
  })
})
