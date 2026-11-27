import { act, renderHook } from '@testing-library/react'
import { beforeEach, describe, expect, it } from 'vitest'
import type { LedgerRow } from '@/features/agent/api/chat-rows'
import { useChatShells } from '@/features/agent/hooks/use-chat-shells'
import { applyFeedFrame, chatStore, openFeed } from '@/features/agent/stores/chat-store'
import { ledger } from '@/__tests__/__fixtures__/ledger-rows'

const push = (row: LedgerRow) => act(() => void applyFeedFrame('chat-1', { type: 'row', row }))

beforeEach(() => {
  chatStore.setState({ feeds: {} })
  openFeed('chat-1')
  applyFeedFrame('chat-1', { type: 'ready', epoch: 'e1' })
})

describe('useChatShells', () => {
  it('is empty for a chat with no shells and follows each start and end', () => {
    const l = ledger()
    const { result } = renderHook(() => useChatShells('chat-1'))
    expect(result.current).toEqual([])

    push(l.start('turn-1'))
    push(
      l.row(
        'shell',
        { phase: 'start', shellId: 'sh-1', command: 'make dev', label: 'Dev' },
        { turnId: 'turn-1' },
      ),
    )
    expect(result.current).toMatchObject([
      { shellId: 'sh-1', command: 'make dev', label: 'Dev', status: 'running' },
    ])

    const before = result.current
    push(l.row('shell', { phase: 'end', shellId: 'sh-1', status: 'stopped' }, { turnId: 'turn-1' }))
    expect(result.current).toMatchObject([{ status: 'stopped' }])
    expect(result.current).not.toBe(before)
  })

  it('keeps its list identity while nothing about the shells changes', () => {
    const l = ledger()
    const { result } = renderHook(() => useChatShells('chat-1'))
    push(l.start('turn-1'))
    push(l.row('shell', { phase: 'start', shellId: 'sh-1' }, { turnId: 'turn-1' }))
    const shells = result.current

    push(l.text('turn-1', 'm1', -1, 'hello'))

    expect(result.current).toBe(shells)
  })
})
