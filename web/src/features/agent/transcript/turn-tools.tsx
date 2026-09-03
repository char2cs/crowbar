import type { AgentToolCall } from '@/features/agent/api/agent-api'
import { describeTool, formatDuration } from '@/features/agent/lib/agent-activity'

/** Tool rows shown under a reply before the rest collapse into a count. */
const LIMIT = 6

/** Finished tool calls, grouped by turn and sorted by seq within each turn.
 *  Computed once per activity change (see agent-transcript.tsx), not once
 *  per row — the O(n) filter+sort this used to do inside every row's render
 *  was quadratic in conversation length. */
export function groupToolCallsByTurn(toolCalls: AgentToolCall[]): Map<string, AgentToolCall[]> {
  const byTurn = new Map<string, AgentToolCall[]>()
  for (const call of toolCalls) {
    if (call.status === 'running') continue
    const list = byTurn.get(call.turnId)
    if (list) list.push(call)
    else byTurn.set(call.turnId, [call])
  }
  for (const list of byTurn.values()) {
    list.sort((a, b) => a.seq - b.seq)
  }
  return byTurn
}

/**
 * What the agent DID to produce a reply, under the reply.
 *
 * Finished calls only — anything still running belongs to the working line, not
 * to a turn that has already been answered.
 */
export function AgentTurnTools({
  callsByTurn,
  turnId,
}: {
  callsByTurn: Map<string, AgentToolCall[]>
  turnId: string
}) {
  if (!turnId) return null
  const calls = callsByTurn.get(turnId) ?? []
  if (calls.length === 0) return null

  return (
    <ul className="tools" data-testid="agent-turn-tools">
      {calls.slice(0, LIMIT).map((call) => (
        <li key={call.id} data-status={call.status}>
          <span>{describeTool(call)}</span>
          {call.durationMs !== undefined && <span>{formatDuration(call.durationMs)}</span>}
        </li>
      ))}
      {calls.length > LIMIT && <li>+{calls.length - LIMIT} more</li>}
    </ul>
  )
}
