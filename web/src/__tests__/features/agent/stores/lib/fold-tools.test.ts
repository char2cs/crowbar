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

  // The call's row names a file inside the workspace from there; the ask's raw path does not.
  it('names the file an ask gates as the gated call’s row names it', () => {
    const l = ledger()
    const feed = applyRows(emptyFeed(), [
      l.start('turn-1'),
      l.row(
        'tool_call',
        { toolId: 'c1', phase: 'pre', name: 'Edit', kind: 'edit', target: 'src/a.go' },
        { turnId: 'turn-1' },
      ),
      l.row(
        'ask',
        {
          askId: 'q1',
          event: 'permission',
          toolId: 'c1',
          toolName: 'Edit',
          toolTarget: '/ws/repo/src/a.go',
          answerable: true,
        },
        { turnId: 'turn-1' },
      ),
    ])
    expect((feed.items.get('a:q1') as AskItem).toolTarget).toBe('src/a.go')
  })

  // codex says why its sandbox asks ("Allow the exact curl command to access example.com…").
  it('keeps the agent’s own sentence on why it asks', () => {
    const l = ledger()
    const why = 'Allow Git to update its read-only metadata so I can commit?'
    const feed = applyRows(emptyFeed(), [
      l.start('turn-1'),
      l.row(
        'ask',
        {
          askId: 'q1',
          event: 'permission',
          toolName: 'commandExecution',
          detail: why,
          answerable: true,
        },
        { turnId: 'turn-1' },
      ),
    ])
    expect((feed.items.get('a:q1') as AskItem).detail).toBe(why)
  })
})
