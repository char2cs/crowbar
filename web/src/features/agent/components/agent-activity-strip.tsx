import { FlickerSpinner } from '@/components/ui/flicker-spinner'
import type { AgentActivity } from '@/features/agent/api/agent-api'
import {
  blockedOn,
  describeInterruption,
  describeTool,
  formatDuration,
  pendingChoices,
  runningSubagents,
  runningTools,
} from '@/features/agent/lib/agent-activity'

/** How many running tools are named before the rest are counted. A turn can fan
 *  out widely, and a list longer than this stops being scannable. */
const MAX_NAMED = 3

/** What the agent is doing right now, in place of the word "working".
 *
 *  This exists because of three failures observed live: a provider blocked on a
 *  trust prompt rendered as silence, a usage limit rendered as an indefinite
 *  spinner, and minutes of tool work rendered as "working…". Every line here is
 *  something the provider actually reported — nothing is estimated, and a
 *  provider that reports nothing renders exactly the spinner it did before.
 */
export function AgentActivityStrip({
  activity,
  working,
  providerLabel,
}: {
  activity: AgentActivity
  working: boolean
  providerLabel: string
}) {
  // A prompt waiting on a human SUPERSEDES everything below it. It is the same
  // event the interruption banner announces — the permission hook records both —
  // drawn as the question it actually is, with the answers on it; two banners for
  // one blockage would just be one of them repeating the other with less. The
  // prompt itself is AgentChoicePrompts, rendered beside this strip, and a chat
  // waiting on a person is not "working…" whatever the turn state says.
  if (pendingChoices(activity).length > 0) return null

  const blocked = blockedOn(activity)

  // A blocked agent is NOT working, whatever the turn state says, and saying
  // "working…" over a modal nobody can see is the exact failure this replaces.
  if (blocked) {
    return (
      <div
        className="flex items-center gap-2 rounded-lg border border-warning/40 bg-warning/10 px-3 py-2 text-sm"
        role="alert"
        data-testid="agent-interruption"
      >
        <span className="text-warning-foreground">{describeInterruption(blocked)}</span>
      </div>
    )
  }

  if (!working) return null

  const running = runningTools(activity)
  const subagents = runningSubagents(activity)

  return (
    <div
      className="flex flex-col gap-1 py-1 text-muted-foreground text-sm"
      role="status"
      data-testid="agent-activity"
    >
      <div className="flex items-center gap-2">
        <FlickerSpinner className="size-4" />
        <span>{providerLabel} is working…</span>
        {subagents > 0 && (
          <span className="text-xs" data-testid="agent-subagent-count">
            {subagents} subagent{subagents === 1 ? '' : 's'}
          </span>
        )}
      </div>
      {running.length > 0 && (
        <ul className="ml-6 flex flex-col gap-0.5 text-xs" data-testid="agent-running-tools">
          {running.slice(0, MAX_NAMED).map((call) => (
            <li key={call.id} className="truncate">
              {describeTool(call)}
            </li>
          ))}
          {running.length > MAX_NAMED && <li>+{running.length - MAX_NAMED} more</li>}
        </ul>
      )}
    </div>
  )
}

/** The tools a finished turn ran, so a completed answer can be inspected rather
 *  than taken on trust. Absent activity renders nothing at all. */
export function AgentTurnTools({ activity, turnId }: { activity: AgentActivity; turnId: string }) {
  const calls = activity.toolCalls
    .filter((c) => c.turnId === turnId && c.status !== 'running')
    .sort((a, b) => a.seq - b.seq)
  if (calls.length === 0) return null

  return (
    <ul
      className="mt-1 flex flex-col gap-0.5 text-muted-foreground text-xs"
      data-testid="agent-turn-tools"
    >
      {calls.map((call) => (
        <li key={call.id} className="flex items-center gap-2 truncate">
          <span
            className={call.status === 'error' ? 'text-destructive' : ''}
            data-tool-status={call.status}
          >
            {describeTool(call)}
          </span>
          {call.durationMs ? (
            <span className="shrink-0">{formatDuration(call.durationMs)}</span>
          ) : null}
        </li>
      ))}
    </ul>
  )
}
