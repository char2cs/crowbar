import { renderHook, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const { mountXterm, openWorkspaceFile } = vi.hoisted(() => ({
  mountXterm: vi.fn(),
  openWorkspaceFile: vi.fn(async () => {}),
}))
vi.mock('@/features/terminal/lib/mount-xterm', () => ({ mountXterm }))
vi.mock('@/features/files/lib/file-tree-handlers', () => ({ openWorkspaceFile }))
vi.mock('@/features/terminal/utils/resolve-font', () => ({
  resolveTerminalFont: async () => 'monospace',
}))

import { useXtermInstance } from '@/features/terminal/hooks/use-xterm-instance'
import { useSidebarStore, type Repo } from '@/lib/store/sidebar'
import { setActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'
import type { TerminalLinksOptions } from '@/features/terminal/lib/terminal-links'
import { toast } from '@/features/window/stores/toast-store'
import { proxyMarkerError } from '@/lib/api/errors'
import { getHostRuntime, destroyHostRuntime } from '@/features/hosts/stores/host-runtime-registry'
import { pairRemoteHost, resetHostWorld } from '@/__tests__/__fixtures__/remote-host'

const repo = {
  id: 'r1',
  name: 'demo',
  avatarLabel: 'D',
  avatarColor: 'avatar-rose',
  defaultWorkspaceId: 'ws-default',
  localPath: '/disk/repo',
  workspaces: [
    { id: 'ws-terminal', branch: 'a', localPath: '/disk/worktrees/a' },
    { id: 'ws-in-view', branch: 'b', localPath: '/disk/worktrees/b' },
  ],
} as unknown as Repo

// A terminal's file links open and resolve in the workspace the terminal belongs to,
// which is not necessarily the one in view (hidden workspaces keep their terminals).
describe('useXtermInstance file links', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    useSidebarStore.setState({ repos: [repo] })
    setActiveWorkspaceId('ws-in-view')
    mountXterm.mockReturnValue({
      terminal: { options: {}, refresh: vi.fn(), rows: 24 },
      addons: { fitAddon: { fit: vi.fn(), proposeDimensions: () => undefined } },
      dispose: vi.fn(),
    })
  })

  afterEach(() => {
    vi.restoreAllMocks()
    destroyHostRuntime('h1')
    resetHostWorld()
  })

  async function fileLinks(): Promise<TerminalLinksOptions> {
    const container = document.createElement('div')
    container.getBoundingClientRect = () => ({ width: 100, height: 100 }) as DOMRect
    renderHook(() =>
      useXtermInstance({
        sessionId: 's1',
        workspaceId: 'ws-terminal',
        container,
        enabled: true,
        write: vi.fn(),
      }),
    )
    await waitFor(() => expect(mountXterm).toHaveBeenCalled())
    return (
      mountXterm.mock.calls[0] as unknown as [unknown, unknown, { links: TerminalLinksOptions }]
    )[2].links
  }

  it('opens a linked file in the terminal’s own workspace', async () => {
    const links = await fileLinks()

    links.openFile('/disk/worktrees/a/src/index.ts')

    expect(openWorkspaceFile).toHaveBeenCalledExactlyOnceWith('ws-terminal', 'src/index.ts', {
      paneId: undefined,
    })
  })

  it('names the remote host a linked file could not be opened from, and records it', async () => {
    pairRemoteHost('h1', 'Studio Mac')
    const toastError = vi.spyOn(toast, 'error').mockImplementation(() => '')
    openWorkspaceFile.mockRejectedValueOnce(proxyMarkerError('host-unreachable', 'h1'))
    const links = await fileLinks()

    links.openFile('/disk/worktrees/a/src/index.ts')

    await waitFor(() =>
      expect(toastError).toHaveBeenCalledExactlyOnceWith(
        'Couldn’t reach Studio Mac',
        'Check that it is online in Settings → Hosts.',
      ),
    )
    expect(getHostRuntime('h1')?.getState().status).toBe('down')
  })

  it('resolves relative links against the terminal’s own workspace root', async () => {
    const links = await fileLinks()

    expect(links.getRoot()).toBe('/disk/worktrees/a')
  })
})
