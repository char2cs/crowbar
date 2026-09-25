import { describe, expect, it } from 'vitest'
import { describeDormant, describeRung, sessionView } from '@/features/agent/lib/session-status'

describe('sessionView', () => {
  it('is pending until the chat list carries the chat', () => {
    expect(
      sessionView({ known: false, liveRunnerId: 'r1', phase: 'live', exitReason: '' }),
    ).toEqual({ state: 'pending' })
  })

  it('is live whenever a runner is placed, whatever the phase says', () => {
    expect(
      sessionView({ known: true, liveRunnerId: 'r1', phase: 'switching', exitReason: '' }),
    ).toEqual({ state: 'live' })
  })

  it('is starting while the daemon places a CLI', () => {
    for (const phase of ['starting', 'switching'] as const) {
      expect(sessionView({ known: true, liveRunnerId: '', phase, exitReason: '' })).toEqual({
        state: 'starting',
      })
    }
  })

  it('is dormant otherwise, carrying the daemon’s exit reason', () => {
    expect(
      sessionView({ known: true, liveRunnerId: '', phase: 'dormant', exitReason: 'stopped' }),
    ).toEqual({ state: 'dormant', exitReason: 'stopped' })
  })
})

describe('describeDormant', () => {
  it('says why, in the daemon’s own terms, and how to continue', () => {
    expect(describeDormant('daemon_restart')).toMatch(/crowbar restarted/i)
    expect(describeDormant('resume_failed')).toMatch(/transcript/i)
    expect(describeDormant('moved')).toMatch(/another conversation/i)
    expect(describeDormant('displaced')).toMatch(/took the agent off/i)
    expect(describeDormant('spawn_failed')).toMatch(/could not start/i)
    expect(describeDormant('connection_lost')).toMatch(/lost its connection/i)
  })

  it('never guesses at a reason this build does not know', () => {
    expect(describeDormant('something_new')).toBe(
      'The agent is not running. Send a message to continue the conversation.',
    )
  })
})

describe('describeRung', () => {
  it('notes only a revive that fell back to the transcript', () => {
    expect(describeRung({ rung: 'transcript' })).toMatch(/transcript/i)
    expect(describeRung({ rung: 'session' })).toBeUndefined()
    expect(describeRung(undefined)).toBeUndefined()
  })
})
