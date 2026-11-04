import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { ContextMenuItem } from '@/components/ui/context-menu'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { showTerminalUrlMenu } from '@/features/terminal/lib/terminal-url-menu'
import { toast } from '@/features/window/stores/toast-store'
import { __resetWorkspaceScopesForTest, recordWorkspaceScope } from '@/lib/workspace-scope'
import {
  loadProjects,
  pairRemoteHost,
  projectOn,
  resetHostWorld,
} from '@/__tests__/__fixtures__/remote-host'

const shown = vi.hoisted(() => ({
  items: [] as ContextMenuItem[],
  position: null as unknown,
  desktop: true,
}))
const openExternalUrl = vi.hoisted(() => vi.fn(() => Promise.resolve()))

vi.mock('@/lib/crowbar-bridge', () => ({
  isDesktopApp: () => shown.desktop,
  showNativeContextMenu: vi.fn((items: ContextMenuItem[], position: unknown) => {
    shown.items = items
    shown.position = position
    return Promise.resolve()
  }),
}))
vi.mock('@/lib/external-open', () => ({ openExternalUrl }))

const url = 'https://a.test/x'
const at = { x: 12, y: 34 }
const noPane = { workspaceId: 'ws-1' }

afterEach(() => {
  shown.desktop = true
  shown.items = []
  vi.restoreAllMocks()
  openExternalUrl.mockClear()
})

const item = (id: string) => {
  const found = shown.items.find((i) => i.id === id)
  if (!found) throw new Error(`no ${id} item`)
  return found
}

describe('showTerminalUrlMenu', () => {
  it('pops a native menu at the pointer with the three link actions', async () => {
    await showTerminalUrlMenu(url, at, { workspaceId: 'ws-1', paneId: 'pane-1' })

    expect(shown.position).toEqual(at)
    expect(shown.items.map((i) => i.label)).toEqual([
      'Open in Default Browser',
      'Open in Crowbar Browser',
      'Copy Link',
    ])
  })

  // The phone app (and a plain browser) has no OS menu and no Crowbar browser: a link just opens.
  it('outside the desktop app opens the link at once, without a menu', async () => {
    shown.desktop = false
    await showTerminalUrlMenu(url, at, noPane)
    await vi.waitFor(() => expect(openExternalUrl).toHaveBeenCalledExactlyOnceWith(url))
    expect(shown.items).toEqual([])
  })

  it('opens the default browser through the external opener', async () => {
    await showTerminalUrlMenu(url, at, noPane)
    item('open-default-browser').onClick()
    expect(openExternalUrl).toHaveBeenCalledExactlyOnceWith(url)
  })

  it('opens a browser tab in the terminal own pane and workspace', async () => {
    const openContent = vi.fn()
    vi.spyOn(windowPaneStore, 'getState').mockReturnValue({
      bufferActions: { openContent },
    } as unknown as ReturnType<typeof windowPaneStore.getState>)

    await showTerminalUrlMenu(url, at, { workspaceId: 'ws-1', paneId: 'pane-1' })
    item('open-crowbar-browser').onClick()

    expect(openContent).toHaveBeenCalledExactlyOnceWith(
      { type: 'browser', url, workspaceId: 'ws-1' },
      { paneId: 'pane-1' },
    )
  })

  it('disables the Crowbar browser item when the terminal is shown in no pane', async () => {
    await showTerminalUrlMenu(url, at, noPane)
    expect(item('open-crowbar-browser').disabled).toBe(true)
  })

  it('copies the link', async () => {
    const writeText = vi.fn(() => Promise.resolve())
    Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true })

    await showTerminalUrlMenu(url, at, noPane)
    item('copy-link').onClick()

    expect(writeText).toHaveBeenCalledExactlyOnceWith(url)
  })
})

// A dev server a remote agent started listens on the remote machine: the same address here
// is whatever this computer runs there, or nothing.
describe('a loopback link printed in a terminal', () => {
  const ON_BUILD_BOX =
    'This link is on Build box, not this computer — forward the port with ssh -L to open it here'
  const local = 'http://localhost:3000/'

  beforeEach(() => {
    resetHostWorld()
    __resetWorkspaceScopesForTest()
    vi.spyOn(toast, 'info').mockReturnValue('')
    pairRemoteHost('h1', 'Build box')
    loadProjects(projectOn('p-remote', 'h1'), projectOn('p-local', 'local'))
    recordWorkspaceScope({ projectId: 'p-remote', repoId: 'r', wsId: 'w-remote' })
    recordWorkspaceScope({ projectId: 'p-local', repoId: 'r', wsId: 'w-local' })
  })

  afterEach(() => resetHostWorld())

  it.each([
    'http://localhost:3000/',
    'http://app.localhost:5173/x',
    'http://127.1.2.3/',
    'http://0.0.0.0:9000',
    'http://[::1]:5173/x',
  ])('to %s from a remote workspace names its host and opens nothing here', async (uri) => {
    await showTerminalUrlMenu(uri, at, { workspaceId: 'w-remote' })
    item('open-default-browser').onClick()
    await vi.waitFor(() => expect(toast.info).toHaveBeenCalledExactlyOnceWith(ON_BUILD_BOX))
    expect(openExternalUrl).not.toHaveBeenCalled()
  })

  it('from a remote workspace opens no Crowbar browser tab either', async () => {
    const openContent = vi.fn()
    vi.spyOn(windowPaneStore, 'getState').mockReturnValue({
      bufferActions: { openContent },
    } as unknown as ReturnType<typeof windowPaneStore.getState>)

    await showTerminalUrlMenu(local, at, { workspaceId: 'w-remote', paneId: 'pane-1' })
    item('open-crowbar-browser').onClick()

    expect(openContent).not.toHaveBeenCalled()
    expect(toast.info).toHaveBeenCalledExactlyOnceWith(ON_BUILD_BOX)
  })

  it('from a Local workspace opens here', async () => {
    await showTerminalUrlMenu(local, at, { workspaceId: 'w-local' })
    item('open-default-browser').onClick()
    await vi.waitFor(() => expect(openExternalUrl).toHaveBeenCalledExactlyOnceWith(local))
    expect(toast.info).not.toHaveBeenCalled()
  })

  it('from a workspace whose host is not known yet does not open as Local', async () => {
    await showTerminalUrlMenu(local, at, { workspaceId: 'w-unplaced' })
    item('open-default-browser').onClick()
    await vi.waitFor(() => expect(toast.info).toHaveBeenCalledOnce())
    expect(openExternalUrl).not.toHaveBeenCalled()
  })

  it('where no remote was ever paired opens here, as it always did', async () => {
    resetHostWorld()
    await showTerminalUrlMenu(local, at, { workspaceId: 'w-anything' })
    item('open-default-browser').onClick()
    await vi.waitFor(() => expect(openExternalUrl).toHaveBeenCalledExactlyOnceWith(local))
  })
})
