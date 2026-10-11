import { afterEach, describe, expect, it, vi } from 'vitest'

const { toastError } = vi.hoisted(() => ({ toastError: vi.fn() }))
vi.mock('@/features/window/stores/toast-store', () => ({ toast: { error: toastError } }))

import { describeSpawnError, toastSpawnFailure } from '@/features/agent/lib/spawn-error'
import { destroyHostRuntime } from '@/features/hosts/stores/host-runtime-registry'
import { ApiError } from '@/lib/api'
import { proxyMarkerError } from '@/lib/api/errors'
import { pairRemoteHost, resetHostWorld } from '@/__tests__/__fixtures__/remote-host'

describe('describeSpawnError', () => {
  it('names the missing CLI on a 424 and tells the user what to do about it', () => {
    const notice = describeSpawnError(
      new ApiError('terminal: command not found: claude', 424),
      'Claude Code',
      'start',
    )

    expect(notice.message).toContain('Claude Code')
    expect(notice.message).toMatch(/isn.t installed/)
    expect(notice.description).toMatch(/PATH/)
  })

  it('does NOT blame the PATH for an unrelated failure', () => {
    // The old copy told the user to check their PATH no matter WHAT went wrong, which
    // sends them hunting for a problem they do not have.
    const notice = describeSpawnError(new ApiError('workspace is locked', 409), 'Codex', 'start')

    expect(notice.description).not.toMatch(/PATH/)
    expect(notice.description).toContain('workspace is locked')
    expect(notice.message).toContain('Codex')
  })

  it('carries the verb of the action that failed', () => {
    const resumed = describeSpawnError(new Error('boom'), 'Codex', 'resume')
    expect(resumed.message).toBe('Couldn’t resume Codex chat')
  })

  it('survives a non-Error rejection', () => {
    const notice = describeSpawnError('kaboom', 'Claude Code', 'start')
    expect(notice.description).toContain('kaboom')
  })
})

describe('toastSpawnFailure', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    toastError.mockReset()
    destroyHostRuntime('h1')
    resetHostWorld()
  })

  // A chat on a remote host that cannot be reached is not a CLI the user should go install.
  it('names the host a spawn could not reach instead of the chat it could not start', () => {
    pairRemoteHost('h1', 'Studio Mac')
    vi.spyOn(console, 'error').mockImplementation(() => {})

    toastSpawnFailure(proxyMarkerError('host-unreachable', 'h1'), 'Codex', 'start')

    expect(toastError).toHaveBeenCalledExactlyOnceWith(
      'Couldn’t reach Studio Mac',
      'Check that it is online in Settings → Hosts.',
    )
  })

  it('says a local spawn failed exactly as before', () => {
    vi.spyOn(console, 'error').mockImplementation(() => {})

    toastSpawnFailure(
      new ApiError('workspace is locked', 409, undefined, undefined, 'local'),
      'Codex',
      'start',
    )

    expect(toastError).toHaveBeenCalledExactlyOnceWith(
      'Couldn’t start Codex chat',
      'workspace is locked',
    )
  })
})
