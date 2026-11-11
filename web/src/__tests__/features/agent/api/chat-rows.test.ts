import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'
import type { RowKind } from '@/features/agent/api/chat-rows'

// Record<RowKind, true> fails to compile unless it names every RowKind and no
// other, so this list is tied to the type; the fixture ties it to the daemon.
const rowKinds: Record<RowKind, true> = {
  user_message: true,
  turn_started: true,
  prompt_settled: true,
  turn_ended: true,
  turn_failed: true,
  session_bound: true,
  session_lost: true,
  marker: true,
  runner_started: true,
  runner_exited: true,
  assistant_text: true,
  reasoning: true,
  tool_call: true,
  ask: true,
  ask_resolved: true,
  subagent: true,
  plan: true,
  move: true,
}

describe('LedgerRow', () => {
  it('names exactly the row kinds the daemon accepts', () => {
    const fixture = resolve(import.meta.dirname, '../../../__fixtures__/ledger-kinds.json')
    const { rowKinds: daemon } = JSON.parse(readFileSync(fixture, 'utf-8')) as {
      rowKinds: string[]
    }
    expect(Object.keys(rowKinds).sort()).toEqual(daemon)
  })
})
