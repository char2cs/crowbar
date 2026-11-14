import { renderHook, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const { mountXterm, revealInEditor } = vi.hoisted(() => ({
  mountXterm: vi.fn(),
  revealInEditor: vi.fn(async () => null),
}))
vi.mock('@/features/terminal/lib/mount-xterm', () => ({ mountXterm }))
vi.mock('@/features/editor/lib/reveal', () => ({ revealInEditor }))
vi.mock('@/features/terminal/utils/resolve-font', () => ({
  resolveTerminalFont: async () => 'monospace',
}))

import { useXtermInstance } from '@/features/terminal/hooks/use-xterm-instance'
import { setActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'
import type { TerminalLinksOptions } from '@/features/terminal/lib/terminal-links'
import type { Repo } from '@/core/entities/repo-types'
import { seedRepos } from '@/__tests__/__fixtures__/sidebar-entities'

const repo = {
  id: 'r1',
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
    seedRepos([repo])
    setActiveWorkspaceId('ws-in-view')
    mountXterm.mockReturnValue({
      terminal: { options: {}, refresh: vi.fn(), rows: 24 },
      addons: { fitAddon: { fit: vi.fn(), proposeDimensions: () => undefined } },
      dispose: vi.fn(),
    })
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

    expect(revealInEditor).toHaveBeenCalledExactlyOnceWith({
      doc: { authority: 'workspace', workspaceId: 'ws-terminal', path: 'src/index.ts' },
      position: undefined,
    })
  })

  it('resolves relative links against the terminal’s own workspace root', async () => {
    const links = await fileLinks()

    expect(links.getRoot()).toBe('/disk/worktrees/a')
  })
})
