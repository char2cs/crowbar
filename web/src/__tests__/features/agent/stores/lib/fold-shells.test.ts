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

  it('keeps what the provider says of a monitor and its command, and follows its lines', () => {
    const l = ledger()
    const turn = l.start('turn-1')
    const monitor = l.row(
      'shell',
      {
        phase: 'start',
        shellId: 'mon-1',
        kind: 'monitor',
        command: 'tail -f app.log',
        label: 'Watch the log',
        cwd: '/work',
        ref: '4242',
        persistent: true,
        timeoutMs: 300000,
      },
      { turnId: 'turn-1' },
    )
    const line = (tail: string) =>
      l.row('shell', { phase: 'note', shellId: 'mon-1', tail }, { turnId: 'turn-1' })
    const feed = applyRows(emptyFeed(), [turn, monitor, line('one'), line('two')])

    expect(shellsOf(feed)).toMatchObject([
      {
        shellKind: 'monitor',
        command: 'tail -f app.log',
        cwd: '/work',
        ref: '4242',
        persistent: true,
        timeoutMs: 300000,
        status: 'running',
        tail: 'two',
      },
    ])
    expect(shellsOf(feed)[0]?.exitCode).toBeUndefined()
  })

  it('reads a start that names no kind as a shell and ends with its exit code and output', () => {
    const l = ledger()
    const turn = l.start('turn-1')
    const started = start(l)
    const done = l.row(
      'shell',
      {
        phase: 'end',
        shellId: 'sh-1',
        status: 'failed',
        exitCode: 2,
        tail: 'boom',
        summary: 'it broke',
      },
      { turnId: 'turn-1' },
    )
    const feed = applyRows(emptyFeed(), [turn, started, done])

    expect(shellsOf(feed)).toMatchObject([
      { shellKind: 'shell', status: 'failed', exitCode: 2, tail: 'boom', summary: 'it broke' },
    ])
  })

  it('ignores a line of a shell that ended', () => {
    const l = ledger()
    const feed = applyRows(emptyFeed(), [
      l.start('turn-1'),
      start(l),
      end(l, 'finished'),
      l.row('shell', { phase: 'note', shellId: 'sh-1', tail: 'late' }, { turnId: 'turn-1' }),
    ])

    expect(shellsOf(feed)[0]?.tail).toBeUndefined()
  })
})
