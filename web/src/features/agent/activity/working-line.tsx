import { useEffect, useState } from 'react'
import { FlickerSpinner } from '@/components/ui/flicker-spinner'
import type { AgentActivity } from '@/features/agent/api/agent-api'
import {
  blockedOn,
  describeTool,
  pendingChoices,
  runningTools,
} from '@/features/agent/lib/agent-activity'
import { VERB_ROTATION_MS, verbAt } from '@/features/agent/activity/lib/verbs'

/** Tool rows shown before the rest collapse into a count. */
const TOOL_LIMIT = 3

interface WorkingLineProps {
  activity: AgentActivity
  working: boolean
  /** When the current turn started, for the elapsed clock. */
  since?: string
}

/**
 * What the agent is doing, while it is doing it.
 *
 * The verb is CROWBAR'S — no provider reports one, so it is flavour, and it must
 * never wear a provider's name. What is real beside it is the elapsed clock and
 * the running tool calls, both of which come straight off the activity feed.
 *
 * It goes quiet the moment the chat is blocked on a person: a chat waiting for
 * an answer is not working, and saying otherwise is how a blocked agent came to
 * look busy.
 */
export function WorkingLine({ activity, working, since }: WorkingLineProps) {
  const [tick, setTick] = useState(0)
  const [elapsed, setElapsed] = useState(0)

  // Compaction rides the same interruption channel as a permission wait, but
  // it isn't blocked on a PERSON — it's the CLI's own housekeeping, and it's a
  // specific, known operation rather than the vague busywork the rotating
  // verb list exists for. Fixed at "Compacting" instead.
  const interruption = blockedOn(activity)
  const compacting = interruption?.kind === 'compaction'

  useEffect(() => {
    if (!working || compacting) {
      setTick(0)
      return
    }
    const timer = window.setInterval(() => setTick((n) => n + 1), VERB_ROTATION_MS)
    return () => window.clearInterval(timer)
  }, [working, compacting])

  useEffect(() => {
    if (!working || !since) {
      setElapsed(0)
      return
    }
    const startedAt = Date.parse(since)
    if (Number.isNaN(startedAt)) return
    const read = () => setElapsed(Math.max(0, Math.round((Date.now() - startedAt) / 1000)))
    read()
    const timer = window.setInterval(read, 1000)
    return () => window.clearInterval(timer)
  }, [working, since])

  // A CHAT WAITING ON A PERSON IS NOT WORKING. The bar already says what is
  // wanted and what to do about it, and a "working…" line beside it is a second
  // voice contradicting the first — which is exactly how a blocked agent came to
  // look busy. Quiet for a prompt nobody can answer too: the fact that the
  // answer has to happen in a terminal does not make this a turn in flight.
  // Compaction is the one interruption kind that doesn't apply here.
  const blocked = pendingChoices(activity).length > 0 || (interruption !== null && !compacting)
  if (!working || blocked) return null
  // Compaction isn't a tool call — nothing to enumerate under it.
  const tools = compacting ? [] : runningTools(activity)

  return (
    <div className="activity" data-testid="agent-activity-strip">
      <div className="hd">
        <FlickerSpinner className="size-4" />
        <span>
          <b className="verb">{compacting ? 'Compacting' : verbAt(tick)}…</b>
          {elapsed > 0 && <span className="dim"> · {elapsed}s</span>}
        </span>
      </div>
      {tools.length > 0 && (
        <ul>
          {tools.slice(0, TOOL_LIMIT).map((call) => (
            <li key={call.id}>{describeTool(call)}</li>
          ))}
          {tools.length > TOOL_LIMIT && <li>+{tools.length - TOOL_LIMIT} more</li>}
        </ul>
      )}
    </div>
  )
}
