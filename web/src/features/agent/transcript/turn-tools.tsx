import type { AgentActivity } from '@/features/agent/api/agent-api'
import { describeTool, formatDuration } from '@/features/agent/lib/agent-activity'

/** Tool rows shown under a reply before the rest collapse into a count. */
const LIMIT = 6

/**
 * What the agent DID to produce a reply, under the reply.
 *
 * Finished calls only — anything still running belongs to the working line, not
 * to a turn that has already been answered.
 */
export function AgentTurnTools({ activity, turnId }: { activity: AgentActivity; turnId: string }) {
  if (!turnId) return null
  const calls = activity.toolCalls
    .filter((call) => call.turnId === turnId && call.status !== 'running')
    .sort((a, b) => a.seq - b.seq)
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
