import { afterEach, describe, expect, it, vi } from 'vitest'

const { toastError } = vi.hoisted(() => ({ toastError: vi.fn() }))
vi.mock('@/features/window/stores/toast-store', () => ({ toast: { error: toastError } }))

import { toastSpawnFailure } from '@/features/agent/lib/spawn-error'
import { destroyHostRuntime } from '@/features/hosts/stores/host-runtime-registry'
import { proxyMarkerError } from '@/lib/api/errors'
import { ApiError } from '@/lib/errors/api-error'
import { UNREACHED } from '@/lib/errors/report'
import { pairRemoteHost, resetHostWorld } from '@/__tests__/__fixtures__/remote-host'

const UNEXPLAINED = 'Something went wrong — details are in the log'

function noticeFor(err: unknown, providerName: string, action: string) {
  vi.spyOn(console, 'error').mockImplementation(() => {})
  toastSpawnFailure(err, providerName, action)
  const [message, description] = toastError.mock.lastCall as [string, string]
  return { message, description }
}

describe('spawn failure notice', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    toastError.mockReset()
  })

  it('names the missing CLI on a 424 and tells the user what to do about it', () => {
    const notice = noticeFor(
      new ApiError('terminal: command not found: claude', 424),
      'Claude Code',
      'start',
    )

    expect(notice.message).toContain('Claude Code')
    expect(notice.message).toMatch(/isn.t installed/)
    expect(notice.description).toMatch(/PATH/)
  })

  it('does NOT blame the PATH for an unrelated failure', () => {
    const notice = noticeFor(new ApiError('workspace is locked', 409), 'Codex', 'start')

    expect(notice.description).not.toMatch(/PATH/)
    expect(notice.description).toContain('workspace is locked')
    expect(notice.message).toContain('Codex')
  })

  it('says calmly that a working chat cannot switch, instead of echoing the refusal', () => {
    const notice = noticeFor(
      new ApiError('agent: chat is busy: conflict', 409, 'chat_busy'),
      'Codex',
      'switch to',
    )

    expect(notice.message).toBe('Can’t switch to Codex while the chat is working')
    expect(notice.description).not.toMatch(/conflict/)
  })

  it('says the session was lost for a session_lost refusal, with no raw daemon text', () => {
    const notice = noticeFor(
      new ApiError(
        'agent: the provider no longer has this chat’s session: conflict',
        409,
        'session_lost',
      ),
      'Codex',
      'resume',
    )
    expect(notice).toEqual({
      message: 'The session was lost',
      description: 'Start a new conversation to continue.',
    })
  })

  it('carries the verb of the action that failed', () => {
    const resumed = noticeFor(new Error('boom'), 'Codex', 'resume')
    expect(resumed.message).toBe('Couldn’t resume Codex chat')
  })

  it('keeps a client bug’s own text out of the notice', () => {
    const notice = noticeFor(
      new TypeError("cannot read properties of undefined (reading 'id')"),
      'Codex',
      'switch to',
    )
    expect(notice.description).toBe(UNEXPLAINED)
  })

  it('says a non-Error rejection reached no daemon', () => {
    const notice = noticeFor('kaboom', 'Claude Code', 'start')
    expect(notice.description).toBe(UNREACHED)
  })
})

describe('toastSpawnFailure', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    toastError.mockReset()
    destroyHostRuntime('h1')
    resetHostWorld()
  })

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
