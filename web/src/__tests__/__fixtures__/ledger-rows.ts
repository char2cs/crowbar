import type { FeedDelta, LedgerRow, RowBodies, RowKind } from '@/features/agent/api/chat-rows'

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
