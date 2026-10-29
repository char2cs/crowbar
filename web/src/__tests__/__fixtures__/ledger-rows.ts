import type { AgentChoice } from '@/features/agent/api/agent-chat-activity'
import type { FeedDelta, LedgerRow, RowBodies, RowKind } from '@/features/agent/api/chat-rows'
import { applyFeedFrame } from '@/features/agent/stores/chat-store'
import { askItemId, type AskItem } from '@/features/agent/stores/lib/transcript-items'

/** Builds a chat's rows with consecutive seqs, as the daemon numbers them. */
export function ledger(chatId = 'chat-1', from = 1) {
  let seq = from - 1
  const at = (n: number) => new Date(Date.UTC(2026, 9, 1, 12, 0, n)).toISOString()
  function row<K extends RowKind>(
    kind: K,
    body: RowBodies[K],
    extra: { turnId?: string; scope?: string } = {},
  ): LedgerRow<K> {
    seq += 1
    return { chatId, seq, kind, at: at(seq), body, ...extra }
  }
  return {
    row,
    get seq() {
      return seq
    },
    prompt: (requestId: string, text: string) =>
      row('user_message', { requestId, text, origin: 'crowbar' }),
    start: (turnId: string, requestId?: string) =>
      row('turn_started', { requestId, provider: 'fake' }, { turnId }),
    text: (turnId: string, messageId: string, index: number, text: string, final = false) =>
      row('assistant_text', { messageId, index, text, final }, { turnId }),
    tool: (turnId: string, toolId: string, phase: 'pre' | 'post' | 'fail', name = 'Read') =>
      row('tool_call', { toolId, phase, name, kind: 'read' }, { turnId }),
    end: (turnId: string, status: 'done' | 'interrupted' = 'done') =>
      row('turn_ended', { status }, { turnId }),
    settle: (requestId: string, consumed = true, reason = '') =>
      row('prompt_settled', { requestId, consumed, reason }),
  }
}

export function delta(
  id: string,
  n: number,
  text: string,
  extra: Partial<FeedDelta> = {},
): FeedDelta {
  return { id, turnId: 'turn-1', stream: 'answer', n, text, ...extra }
}

/** A folded ask as the composer receives it; a pending permission by default. */
export function askItem(choice: Partial<AgentChoice> = {}, toolTarget = ''): AskItem {
  const id = choice.id ?? 'k1'
  return {
    kind: 'ask',
    id: askItemId(id),
    seq: choice.seq ?? 1,
    turnId: choice.turnId ?? 'turn-1',
    at: choice.at ?? '2026-08-18T12:00:00Z',
    scope: '',
    toolTarget,
    choice: {
      id,
      turnId: 'turn-1',
      seq: 1,
      kind: 'tool_permission',
      toolName: 'Bash',
      options: [{ id: 'allow', kind: 'allow', label: 'Allow' }],
      pending: true,
      answerable: true,
      at: '2026-08-18T12:00:00Z',
      ...choice,
    },
  }
}

/** Puts a chat's feed in the store as a replay of `rows` would leave it, live. */
export function seedFeed(chatId: string, rows: LedgerRow[] = []) {
  for (const row of rows) applyFeedFrame(chatId, { type: 'row', row })
  applyFeedFrame(chatId, { type: 'ready', epoch: 'test' })
}

/** One finished turn with one reply: a chat that has been spoken in. */
export function spokenIn(chatId: string, reply = 'earlier turn'): LedgerRow[] {
  const l = ledger(chatId)
  return [l.start('turn-1'), l.text('turn-1', 'm1', 0, reply, true), l.end('turn-1')]
}
