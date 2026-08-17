import type {
  AgentActivity,
  AgentInterruption,
  AgentToolCall,
} from '@/features/agent/api/agent-api'

/** The empty activity, so a caller never has to null-check three lists. */
export const NO_ACTIVITY: AgentActivity = { toolCalls: [], subagents: [], interruptions: [] }

/** An interruption the agent is still blocked on.
 *
 *  Only the LATEST unresolved one is surfaced: they are states, not a log, and a
 *  stack of "waiting for permission" banners tells a user nothing the top one
 *  does not. */
export function blockedOn(activity: AgentActivity): AgentInterruption | null {
  const open = activity.interruptions.filter((i) => !i.resolvedAt)
  if (open.length === 0) return null
  return open.reduce((latest, next) => (next.seq > latest.seq ? next : latest))
}

/** Tool calls still running, oldest first — which is the order they started and
 *  the order a reader scans. */
export function runningTools(activity: AgentActivity): AgentToolCall[] {
  return activity.toolCalls.filter((c) => c.status === 'running').sort((a, b) => a.seq - b.seq)
}

/** Subagents still working. Starts and stops do NOT balance on either provider —
 *  a stop also fires for anonymous internal subagents — so this counts what has
 *  a start and no end, and never tries to reconcile the two populations. */
export function runningSubagents(activity: AgentActivity): number {
  return activity.subagents.filter((s) => !s.endedAt).length
}

/** How a tool call reads in one line: the tool, and what it acted on when the
 *  provider said so. */
export function describeTool(call: AgentToolCall): string {
  if (!call.target) return call.name
  return `${call.name} · ${call.target}`
}

/** Human copy for why the agent is stopped. Each kind is a genuinely different
 *  thing to tell someone, which is why they are not collapsed into one string. */
export function describeInterruption(interruption: AgentInterruption): string {
  switch (interruption.kind) {
    case 'permission':
      return interruption.detail
        ? `Waiting for your permission to run ${interruption.detail}`
        : 'Waiting for your permission'
    case 'notification':
      return interruption.detail || 'The agent needs your attention'
    case 'elicitation':
      return interruption.detail || 'The agent is waiting for input'
    case 'compaction':
      return 'Compacting the conversation to free context'
    default:
      return interruption.detail || 'The agent is waiting'
  }
}

/** A duration a person can read at a glance. Sub-second work is reported in
 *  milliseconds because "0s" reads as "nothing happened". */
export function formatDuration(ms: number | undefined): string {
  if (!ms || ms < 0) return ''
  if (ms < 1000) return `${ms}ms`
  if (ms < 60_000) return `${(ms / 1000).toFixed(1)}s`
  const minutes = Math.floor(ms / 60_000)
  const seconds = Math.round((ms % 60_000) / 1000)
  return seconds === 0 ? `${minutes}m` : `${minutes}m ${seconds}s`
}
