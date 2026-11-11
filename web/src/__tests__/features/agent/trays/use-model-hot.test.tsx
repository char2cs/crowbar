import { act, renderHook } from '@testing-library/react'
import { beforeEach, describe, expect, it } from 'vitest'
import type { LedgerRow } from '@/features/agent/api/chat-rows'
import { applyFeedFrame, chatStore, openFeed } from '@/features/agent/stores/chat-store'
import { useModelHot } from '@/features/agent/trays/use-model-hot'
import { ledger } from '@/__tests__/__fixtures__/ledger-rows'

const push = (row: LedgerRow) => act(() => void applyFeedFrame('chat-1', { type: 'row', row }))

beforeEach(() => {
  chatStore.setState({ feeds: {}, outbox: {} })
  openFeed('chat-1')
  applyFeedFrame('chat-1', { type: 'ready', epoch: 'e1' })
})

describe('useModelHot', () => {
  it('lights the model chip after a usage limit, and not after another failure', () => {
    const { result } = renderHook(() => useModelHot('chat-1'))
    expect(result.current).toBe(false)
    const l = ledger()
    push(l.start('turn-1'))
    push(
      l.row(
        'turn_failed',
        { reason: 'rate_limit', detail: '429 usage limit' },
        { turnId: 'turn-1' },
      ),
    )
    expect(result.current).toBe(true)
    push(l.start('turn-2'))
    push(l.row('turn_failed', { reason: 'authentication_failed' }, { turnId: 'turn-2' }))
    expect(result.current).toBe(false)
  })
})
