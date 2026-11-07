import { describe, expect, it } from 'vitest'
import { emptyFeed } from '@/features/agent/stores/lib/fold-rows'
import type { AskItem } from '@/features/agent/stores/lib/transcript-items'
import { applyRows, ledger } from '@/__tests__/__fixtures__/ledger-rows'

describe('applyAsk', () => {
  it('knows the kind of call an ask gates, from that call’s own row', () => {
    const l = ledger()
    const feed = applyRows(emptyFeed(), [
      l.start('turn-1'),
      l.row(
        'tool_call',
        { toolId: 'c1', phase: 'pre', name: 'commandExecution', kind: 'execute' },
        { turnId: 'turn-1' },
      ),
      l.row(
        'ask',
        {
          askId: 'q1',
          event: 'permission',
          toolId: 'c1',
          toolName: 'commandExecution',
          answerable: true,
        },
        { turnId: 'turn-1' },
      ),
    ])
    expect((feed.items.get('a:q1') as AskItem).family).toBe('execute')
  })
})
