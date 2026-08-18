import type {
  AgentActivity,
  AgentChoice,
  AgentInterruption,
  AgentToolCall,
} from '@/features/agent/api/agent-api'

/** The empty activity, so a caller never has to null-check four lists. */
export const NO_ACTIVITY: AgentActivity = {
  toolCalls: [],
  subagents: [],
  interruptions: [],
  choices: [],
}

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

/**
 * The prompts the agent is still waiting on a human to answer, oldest first.
 *
 * Unlike an interruption these are NOT collapsed to the latest one. An
 * interruption is a state and a stack of them tells a reader nothing the top one
 * does not; a prompt is a QUESTION, each is separately answerable, and hiding all
 * but the newest would leave the CLI blocked on one nobody was shown.
 *
 * `pending` is read straight off the server on every poll, which is what makes a
 * prompt answered at the terminal disappear here without this view being told:
 * the terminal can resolve one at any instant, so this is advisory and the next
 * read is the authority.
 */
export function pendingChoices(activity: AgentActivity): AgentChoice[] {
  return activity.choices.filter((choice) => choice.pending).sort((a, b) => a.seq - b.seq)
}

/**
 * What a permission is actually about to touch.
 *
 * A prompt carries the tool's NAME but never its target — the target is reported
 * on the tool call the permission gates, which the provider opened moments
 * earlier in the same turn. '' when no such call is in flight, because a guess
 * about which file is about to be written would be worse than saying nothing.
 */
export function choiceToolTarget(activity: AgentActivity, choice: AgentChoice): string {
  if (!choice.toolName) return ''
  const gated = activity.toolCalls.find(
    (call) =>
      call.turnId === choice.turnId && call.name === choice.toolName && call.status === 'running',
  )
  return gated?.target ?? ''
}

/** The headline of a prompt: what is being asked, in one line.
 *
 *  Each kind is a genuinely different thing to put to someone, which is why they
 *  are not collapsed into one string — and why a kind this build has never heard
 *  of falls through to whatever the provider did say rather than to a guess. */
export function describeChoice(choice: AgentChoice): string {
  switch (choice.kind) {
    case 'tool_permission':
      return choice.toolName ? `Run ${choice.toolName}?` : 'The agent is asking for permission'
    case 'question':
      return choice.question || choice.title || 'The agent has a question'
    case 'elicitation':
      return choice.question || 'A tool is asking for some details'
    default:
      return choice.question || choice.title || 'The agent is waiting for your answer'
  }
}

/** The second line under the headline — the permission's target, or the title the
 *  headline did not already use. '' draws no line at all. */
export function choiceDetail(activity: AgentActivity, choice: AgentChoice): string {
  if (choice.kind === 'tool_permission') return choiceToolTarget(activity, choice)
  const headline = describeChoice(choice)
  return choice.title && choice.title !== headline ? choice.title : ''
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
