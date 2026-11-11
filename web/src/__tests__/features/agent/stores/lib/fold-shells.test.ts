import { describe, expect, it } from 'vitest'
import { emptyFeed } from '@/features/agent/stores/lib/fold-rows'
import { shellsOf } from '@/features/agent/stores/lib/fold-shells'
import { applyRows, ledger } from '@/__tests__/__fixtures__/ledger-rows'

const start = (l: ReturnType<typeof ledger>, shellId = 'sh-1') =>
  l.row(
    'shell',
    { phase: 'start', shellId, command: 'npm run dev', label: 'Dev server' },
    { turnId: 'turn-1' },
  )

const end = (
  l: ReturnType<typeof ledger>,
  status: 'finished' | 'failed' | 'stopped',
  shellId = 'sh-1',
) => l.row('shell', { phase: 'end', shellId, status }, { turnId: 'turn-1' })

describe('background shells', () => {
  it('lists a started shell as running, outside the transcript order', () => {
    const l = ledger()
    const feed = applyRows(emptyFeed(), [l.start('turn-1'), start(l)])

    expect(shellsOf(feed)).toMatchObject([
      { shellId: 'sh-1', command: 'npm run dev', label: 'Dev server', status: 'running' },
    ])
    expect(shellsOf(feed)[0]?.startedAt).toBeTruthy()
    expect(feed.order).toEqual([])
  })

  it('outlives the turn that started it', () => {
    const l = ledger()
    const feed = applyRows(emptyFeed(), [l.start('turn-1'), start(l), l.end('turn-1')])

    expect(shellsOf(feed).map((s) => s.status)).toEqual(['running'])
  })

  it.each(['finished', 'failed', 'stopped'] as const)('ends as %s, once', (status) => {
    const l = ledger()
    const feed = applyRows(emptyFeed(), [
      l.start('turn-1'),
      start(l),
      end(l, status),
      end(l, status === 'failed' ? 'finished' : 'failed'),
    ])

    expect(shellsOf(feed).map((s) => s.status)).toEqual([status])
    expect(shellsOf(feed)[0]?.endedAt).toBeTruthy()
  })

  it('ends nothing for an end that names no started shell', () => {
    const l = ledger()
    const feed = applyRows(emptyFeed(), [l.start('turn-1'), end(l, 'finished', 'unknown')])

    expect(shellsOf(feed)).toEqual([])
  })

  it('stops what is running when the CLI that ran it exits, and keeps what had ended', () => {
    const l = ledger()
    const feed = applyRows(emptyFeed(), [
      l.start('turn-1'),
      start(l, 'sh-1'),
      start(l, 'sh-2'),
      end(l, 'failed', 'sh-2'),
      l.row('runner_exited', { runnerId: 'r1', reason: 'exited', exitCode: 0 }),
    ])

    expect(shellsOf(feed).map((s) => [s.shellId, s.status])).toEqual([
      ['sh-1', 'stopped'],
      ['sh-2', 'failed'],
    ])
  })
})
