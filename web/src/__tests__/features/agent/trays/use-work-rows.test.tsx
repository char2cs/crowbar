import { act, renderHook } from '@testing-library/react'
import { beforeEach, describe, expect, it } from 'vitest'
import type { LedgerRow } from '@/features/agent/api/chat-rows'
import { applyFeedFrame, chatStore, openFeed } from '@/features/agent/stores/chat-store'
import { useWorkRows } from '@/features/agent/trays/use-work-rows'
import { ledger } from '@/__tests__/__fixtures__/ledger-rows'

const push = (row: LedgerRow) => act(() => void applyFeedFrame('chat-1', { type: 'row', row }))

beforeEach(() => {
  chatStore.setState({ feeds: {} })
  openFeed('chat-1')
  applyFeedFrame('chat-1', { type: 'ready', epoch: 'e1' })
})

describe('useWorkRows', () => {
  it('has no rows while nothing runs', () => {
    const { result } = renderHook(() => useWorkRows('chat-1'))
    expect(result.current).toEqual([])
  })

  it('lists each running subagent with its title, current action and start', () => {
    const l = ledger()
    const { result } = renderHook(() => useWorkRows('chat-1'))
    push(l.start('turn-1'))
    push(
      l.row(
        'tool_call',
        { toolId: 'c', phase: 'pre', name: 'Agent', kind: 'subagent', title: 'Scout' },
        { turnId: 'turn-1' },
      ),
    )
    push(
      l.row('subagent', { phase: 'bound', spawnToolId: 'c' }, { turnId: 'turn-1', scope: 'sub-1' }),
    )
    push(
      l.row(
        'tool_call',
        { toolId: 'r', phase: 'pre', kind: 'read', target: '/a/b' },
        { turnId: 'turn-1', scope: 'sub-1' },
      ),
    )
    push(l.row('subagent', { phase: 'start', type: 'plan' }, { turnId: 'turn-1', scope: 'sub-2' }))
    expect(result.current).toHaveLength(2)
    expect(result.current[0]).toMatchObject({
      kind: 'subagent',
      label: 'Scout',
      detail: 'Reading /a/b',
      status: 'running',
    })
    expect(result.current[1]).toMatchObject({ kind: 'subagent', label: 'plan' })
    push(l.row('subagent', { phase: 'stop' }, { turnId: 'turn-1', scope: 'sub-1' }))
    // The one that finished stays beside the one still running, how it ended on its row.
    expect(result.current.map((row) => [row.label, row.status])).toEqual([
      ['plan', 'running'],
      ['Scout', 'done'],
    ])
    expect(result.current[1]?.detail).toMatch(/^done/)
    expect(result.current[1]?.endedAt).toBeTruthy()
    push(l.row('subagent', { phase: 'stop' }, { turnId: 'turn-1', scope: 'sub-2' }))
    expect(result.current).toEqual([])
  })

  it('names a typed subagent by its kind, then what it was given and how it ended', () => {
    const l = ledger()
    const { result } = renderHook(() => useWorkRows('chat-1'))
    push(l.start('turn-1'))
    push(
      l.row(
        'tool_call',
        { toolId: 'c', phase: 'pre', name: 'Agent', kind: 'subagent', title: 'Audit queries' },
        { turnId: 'turn-1' },
      ),
    )
    push(
      l.row(
        'subagent',
        { phase: 'start', spawnToolId: 'c', title: 'Audit queries' },
        {
          turnId: 'turn-1',
        },
      ),
    )
    push(
      l.row(
        'subagent',
        { phase: 'bound', spawnToolId: 'c', type: 'Explore' },
        { turnId: 'turn-1', scope: 'sub-1' },
      ),
    )
    push(l.row('subagent', { phase: 'start', type: 'plan' }, { turnId: 'turn-1', scope: 'sub-2' }))
    expect(result.current[0]).toMatchObject({
      label: 'Explore',
      detail: 'Audit queries',
      hint: 'Thinking…',
    })
    push(l.row('subagent', { phase: 'stop' }, { turnId: 'turn-1', scope: 'sub-1' }))
    expect(result.current.find((row) => row.label === 'Explore')?.detail).toBe(
      'Audit queries · done',
    )
  })

  it('keeps the same rows while unrelated rows arrive', () => {
    const l = ledger()
    const { result } = renderHook(() => useWorkRows('chat-1'))
    push(l.start('turn-1'))
    push(l.row('subagent', { phase: 'start', type: 'plan' }, { turnId: 'turn-1', scope: 'sub-2' }))
    const before = result.current
    push(l.row('user_message', { requestId: 'r1', text: 'hi', origin: 'crowbar' }))
    expect(result.current).toBe(before)
  })
})

describe('useWorkRows shells and monitors', () => {
  const startShell = (l: ReturnType<typeof ledger>, body: Record<string, unknown>) =>
    push(l.row('shell', { phase: 'start', ...body } as never, { turnId: 'turn-1' }))

  it('lists a running shell and a monitor with their words, after the subagents', () => {
    const l = ledger()
    const { result } = renderHook(() => useWorkRows('chat-1'))
    push(l.start('turn-1'))
    startShell(l, {
      shellId: 'sh-1',
      command: 'npm run dev',
      label: 'Dev server',
      cwd: '/work',
      ref: '4242',
    })
    startShell(l, { shellId: 'mon-1', kind: 'monitor', command: 'tail -f app.log' })
    push(l.row('subagent', { phase: 'start', type: 'plan' }, { turnId: 'turn-1', scope: 'sub-1' }))
    push(
      l.row(
        'shell',
        { phase: 'note', shellId: 'mon-1', tail: 'a\nERROR down' },
        { turnId: 'turn-1' },
      ),
    )

    expect(result.current.map((row) => [row.kind, row.label, row.detail, row.status])).toEqual([
      ['subagent', 'plan', 'Thinking…', 'running'],
      ['shell', 'Dev server', 'npm run dev', 'running'],
      ['monitor', 'tail -f app.log', 'ERROR down', 'running'],
    ])
    expect(result.current[1]?.hint).toContain('process 4242')
    expect(result.current[1]?.hint).toContain('in /work')
  })

  it('keeps an ended one with how it ended, behind the running ones', () => {
    const l = ledger()
    const { result } = renderHook(() => useWorkRows('chat-1'))
    push(l.start('turn-1'))
    startShell(l, { shellId: 'sh-1', command: 'make test' })
    startShell(l, { shellId: 'sh-2', command: 'make dev' })
    push(
      l.row(
        'shell',
        { phase: 'end', shellId: 'sh-1', status: 'failed', exitCode: 2, tail: 'x\nboom' },
        { turnId: 'turn-1' },
      ),
    )

    expect(result.current.map((row) => [row.label, row.status, row.detail])).toEqual([
      ['make dev', 'running', 'opened by the agent, still managed'],
      ['make test', 'failed', 'Exit 2 · boom'],
    ])
    push(l.row('shell', { phase: 'end', shellId: 'sh-2', status: 'stopped' }, { turnId: 'turn-1' }))
    expect(result.current.map((row) => [row.status, row.detail])).toEqual([
      ['failed', 'Exit 2 · boom'],
      ['stopped', 'Stopped'],
    ])
  })
})
