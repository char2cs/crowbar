import { useEffect, useState } from 'react'
import { FlickerSpinner } from '@/components/ui/flicker-spinner'
import { formatElapsed } from '@/features/agent/activity/lib/shelf-fit'
import { reasoningHeadline } from '@/features/agent/activity/lib/reasoning-headline'
import { VERB_ROTATION_MS, verbAt } from '@/features/agent/activity/lib/verbs'

interface WorkingLineProps {
  working: boolean
  /** When the current turn started, for the elapsed clock. */
  since?: string
  /** A compaction opens no turn, so it shows here whether or not one is running. */
  compacting?: boolean
  /** What the agent is thinking right now: during a long reasoning stretch it
   *  is the only thing on screen that says the chat is alive. */
  reasoning?: string
  /** The agent's own to-do list, in Crowbar's statuses (pending/active/done). */
  plan?: { text: string; status?: string }[]
  /** An ask is in the composer: the bar is speaking for the wait. */
  asking?: boolean
}

/** What the agent is doing, while it is doing it: its own live words when it
 *  says any, else Crowbar's rotating verb, beside the turn's elapsed clock.
 *  Quiet while an ask holds the composer: a chat waiting on a person is not working. */
export function WorkingLine({
  working,
  since,
  compacting = false,
  reasoning,
  plan,
  asking = false,
}: WorkingLineProps) {
  const [tick, setTick] = useState(0)
  const [elapsed, setElapsed] = useState(0)

  const thought = reasoning ? reasoningHeadline(reasoning) : null

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

  if ((!working && !compacting) || asking) return null

  return (
    <div className="activity" data-testid="agent-activity-strip">
      <div className="hd">
        <FlickerSpinner className="size-4" />
        <span>
          <b className="verb" data-testid="agent-working-heading">
            {compacting ? 'Compacting…' : thought?.headline ? thought.headline : `${verbAt(tick)}…`}
          </b>
          {elapsed > 0 && <span className="dim"> · {formatElapsed(elapsed)}</span>}
        </span>
      </div>
      {thought?.body && !compacting && (
        <p className="reasoning-subtitle" data-testid="agent-reasoning">
          {thought.body}
        </p>
      )}
      {plan && plan.length > 0 && !compacting && (
        <ol className="plan" data-testid="agent-plan">
          {/* Steps carry no id; text with the index keys a changed step fresh. */}
          {plan.map((step, i) => (
            // react-doctor-disable-next-line no-array-index-as-key -- steps carry no id; the text is part of the key
            <li key={`${i}-${step.text}`} data-status={step.status}>
              {step.text}
            </li>
          ))}
        </ol>
      )}
    </div>
  )
}
