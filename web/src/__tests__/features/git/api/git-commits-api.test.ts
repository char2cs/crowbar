import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const { apiFetch, toastError } = vi.hoisted(() => ({ apiFetch: vi.fn(), toastError: vi.fn() }))
vi.mock('@/lib/api', () => ({ apiFetch }))
vi.mock('@/features/window/stores/toast-store', () => ({ toast: { error: toastError } }))

import { commitChanges } from '@/features/git/api/git-commits-api'
import { destroyHostRuntime } from '@/features/hosts/stores/host-runtime-registry'
import { proxyMarkerError } from '@/lib/api/errors'
import { __resetWorkspaceScopesForTest, recordWorkspaceScope } from '@/lib/workspace-scope'
import {
  loadProjects,
  pairRemoteHost,
  projectOn,
  resetHostWorld,
} from '@/__tests__/__fixtures__/remote-host'

// The commit box only learns that a commit failed; a remote host behind the failure is named
// here, where the error still is.
describe('commitChanges', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.spyOn(console, 'error').mockImplementation(() => {})
    __resetWorkspaceScopesForTest()
    recordWorkspaceScope({ projectId: 'p1', repoId: 'r', wsId: 'w1', owningChatId: 'chat-1' })
  })

  afterEach(() => {
    vi.restoreAllMocks()
    destroyHostRuntime('h1')
    resetHostWorld()
  })

  it('names the remote host a commit could not reach', async () => {
    pairRemoteHost('h1', 'Studio Mac')
    loadProjects(projectOn('p1', 'h1'))
    apiFetch.mockRejectedValue(proxyMarkerError('host-unreachable', 'h1'))

    await expect(commitChanges('w1', 'fix it')).resolves.toBe(false)

    expect(apiFetch.mock.calls[0]?.[0]).toBe('crowbar://h1/v0/chats/chat-1/git/commit')
    expect(toastError).toHaveBeenCalledExactlyOnceWith(
      'Couldn’t reach Studio Mac',
      'Check that it is online in Settings → Hosts.',
    )
  })

  it('leaves a local failure to the commit box, as before', async () => {
    apiFetch.mockRejectedValue(new Error('nothing to commit'))

    await expect(commitChanges('w1', 'fix it')).resolves.toBe(false)

    expect(toastError).not.toHaveBeenCalled()
  })
})
