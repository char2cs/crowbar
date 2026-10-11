import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'

vi.mock('@/lib/api', () => ({ apiFetch: vi.fn() }))
vi.mock('@/features/window/stores/toast-store', () => ({
  toast: { error: vi.fn(), success: vi.fn() },
}))

import { apiFetch } from '@/lib/api'
import { toast } from '@/features/window/stores/toast-store'
import { openFileContent } from '@/features/workspace/lib/open-file-content'
import { setWorkspaceScope } from '@/lib/workspace-scope'
import { proxyMarkerError } from '@/lib/api/errors'
import { destroyHostRuntime } from '@/features/hosts/stores/host-runtime-registry'
import {
  loadProjects,
  pairRemoteHost,
  projectOn,
  resetHostWorld,
} from '@/__tests__/__fixtures__/remote-host'

beforeEach(() => {
  vi.clearAllMocks()
  // The content route is addressed through the chat holding the worktree;
  // record a scope carrying one for the test wsId.
  setWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws-1', owningChatId: 'chat-1' })
})

afterEach(() => {
  destroyHostRuntime('h1')
  resetHostWorld()
})

describe('openFileContent', () => {
  it('fetches the chat-scoped content route and opens a buffer', async () => {
    ;(apiFetch as ReturnType<typeof vi.fn>).mockResolvedValue({ content: 'hello world' })
    const openContent = vi.fn()
    await openFileContent('ws-1', 'web/vite.config.ts', { openContent } as never, {
      preview: false,
    })
    expect(apiFetch).toHaveBeenCalledWith(
      '/v0/chats/chat-1/files/content?path=web%2Fvite.config.ts',
    )
    expect(openContent).toHaveBeenCalledWith(
      expect.objectContaining({
        type: 'editor',
        path: 'web/vite.config.ts',
        name: 'vite.config.ts',
        content: 'hello world',
      }),
      { paneId: undefined },
    )
    expect(toast.error).not.toHaveBeenCalled()
  })

  it('decodes base64-encoded content', async () => {
    ;(apiFetch as ReturnType<typeof vi.fn>).mockResolvedValue({
      content: btoa('binary'),
      encoding: 'base64',
    })
    const openContent = vi.fn()
    await openFileContent('ws-1', 'a/b.bin', { openContent } as never, {})
    expect(openContent).toHaveBeenCalledWith(expect.objectContaining({ content: 'binary' }), {
      paneId: undefined,
    })
  })

  it('shows an error toast and opens nothing on failure', async () => {
    ;(apiFetch as ReturnType<typeof vi.fn>).mockRejectedValue(new Error('500'))
    const openContent = vi.fn()
    await openFileContent('ws-1', 'web/vite.config.ts', { openContent } as never, {
      preview: false,
    })
    expect(openContent).not.toHaveBeenCalled()
    expect(toast.error).toHaveBeenCalledWith('Failed to open file', 'vite.config.ts')
  })

  it('names the remote host a file could not be read from', async () => {
    pairRemoteHost('h1', 'Studio Mac')
    loadProjects(projectOn('p1', 'h1'))
    ;(apiFetch as ReturnType<typeof vi.fn>).mockRejectedValue(
      proxyMarkerError('host-unreachable', 'h1'),
    )

    await openFileContent('ws-1', 'web/vite.config.ts', { openContent: vi.fn() } as never)

    expect(toast.error).toHaveBeenCalledExactlyOnceWith(
      'Couldn’t reach Studio Mac',
      'Check that it is online in Settings → Hosts.',
    )
  })

  it('passes isPreview when preview=true', async () => {
    ;(apiFetch as ReturnType<typeof vi.fn>).mockResolvedValue({ content: 'x' })
    const openContent = vi.fn()
    await openFileContent('ws-1', 'a/b.ts', { openContent } as never, {
      preview: true,
      paneId: 'pane-2',
    })
    expect(openContent).toHaveBeenCalledWith(expect.objectContaining({ isPreview: true }), {
      paneId: 'pane-2',
    })
  })
})
