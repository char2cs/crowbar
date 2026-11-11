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
    expect(result.current.map((row) => row.label)).toEqual(['plan'])
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
