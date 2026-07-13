import { describe, expect, it } from 'vitest'

import { describeSpawnError } from '@/features/agent/lib/spawn-error'
import { ApiError } from '@/lib/api'

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
