import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const { openExternalUrl, addons } = vi.hoisted(() => ({
  openExternalUrl: vi.fn(async (_url: string) => {}),
  addons: [] as Array<{ handler: (event: MouseEvent, uri: string) => void }>,
}))
vi.mock('@/lib/external-open', () => ({ openExternalUrl }))
// The addon's link detection needs a rendered xterm; what a click does is the handler it is given.
vi.mock('@xterm/addon-web-links', () => ({
  WebLinksAddon: class {
    constructor(readonly handler: (event: MouseEvent, uri: string) => void) {
      addons.push(this)
    }
  },
}))

import type { Terminal } from '@xterm/xterm'
import {
  injectLinkStyles,
  loadWebLinksAddon,
  removeLinkStyles,
} from '@/features/terminal/hooks/use-terminal-addons'
import { toast } from '@/features/window/stores/toast-store'
import { __resetWorkspaceScopesForTest, recordWorkspaceScope } from '@/lib/workspace-scope'
import {
  loadProjects,
  pairRemoteHost,
  projectOn,
  resetHostWorld,
} from '@/__tests__/__fixtures__/remote-host'

/** Clicks `uri` in a terminal of workspace `wsId`. */
function click(wsId: string, uri: string): void {
  addons.length = 0
  loadWebLinksAddon({ loadAddon: () => {} } as unknown as Terminal, () => wsId)
  addons[0].handler(new MouseEvent('click'), uri)
}

const ON_BUILD_BOX =
  'This link is on Build box, not this computer — forward the port with ssh -L to open it here'

beforeEach(() => {
  resetHostWorld()
  __resetWorkspaceScopesForTest()
  openExternalUrl.mockClear()
  vi.spyOn(toast, 'info').mockReturnValue('')
  pairRemoteHost('h1', 'Build box')
  loadProjects(projectOn('p-remote', 'h1'), projectOn('p-local', 'local'))
  recordWorkspaceScope({ projectId: 'p-remote', repoId: 'r', wsId: 'w-remote', owningChatId: 'c1' })
  recordWorkspaceScope({ projectId: 'p-local', repoId: 'r', wsId: 'w-local', owningChatId: 'c2' })
})

afterEach(() => {
  vi.restoreAllMocks()
})

// A dev server a remote agent started listens on the remote machine: the same address here
// is whatever this computer runs there, or nothing.
describe('a terminal link', () => {
  it.each([
    'http://localhost:3000/',
    'http://LOCALHOST:3000',
    'http://app.localhost:5173/x',
    'http://127.0.0.1:8080',
    'http://127.1.2.3/',
    'http://0.0.0.0:9000',
    'http://[::1]:5173/x',
  ])('to %s from a remote workspace names its host, and opens nothing here', (uri) => {
    click('w-remote', uri)

    expect(openExternalUrl).not.toHaveBeenCalled()
    expect(toast.info).toHaveBeenCalledExactlyOnceWith(ON_BUILD_BOX)
  })

  it('to localhost from a Local workspace opens here', () => {
    click('w-local', 'http://localhost:3000/')

    expect(openExternalUrl).toHaveBeenCalledExactlyOnceWith('http://localhost:3000/')
    expect(toast.info).not.toHaveBeenCalled()
  })

  it('to a public address from a remote workspace opens here', () => {
    click('w-remote', 'https://example.com/docs')

    expect(openExternalUrl).toHaveBeenCalledExactlyOnceWith('https://example.com/docs')
    expect(toast.info).not.toHaveBeenCalled()
  })

  it('to localhost from a workspace whose host is not known yet does not open as Local', () => {
    click('w-unplaced', 'http://localhost:3000/')

    expect(openExternalUrl).not.toHaveBeenCalled()
    expect(toast.info).toHaveBeenCalledOnce()
  })

  it('to localhost where no remote was ever paired opens here, as it always did', () => {
    resetHostWorld()
    click('w-anything', 'http://localhost:3000/')

    expect(openExternalUrl).toHaveBeenCalledExactlyOnceWith('http://localhost:3000/')
  })
})

// An agent pane's container id carries the PTY id its daemon chose, maybe on another host.
describe('terminal link styles', () => {
  it('stay scoped to the terminal’s own container whatever its id holds', () => {
    const containerId = 'terminal-x,*{display:none} #y'
    injectLinkStyles('pty', containerId)
    const sheet = (document.getElementById('terminal-link-style-pty') as HTMLStyleElement).sheet!
    const rules = Array.from(sheet.cssRules) as CSSStyleRule[]

    expect(rules).toHaveLength(2)
    for (const rule of rules) {
      expect(rule.selectorText.startsWith(`#${CSS.escape(containerId)} `)).toBe(true)
      expect(rule.style.display).toBe('')
    }
    removeLinkStyles('pty')
  })
})
