import { useEffect, useState } from 'react'
import { FlickerSpinner } from '@/components/ui/flicker-spinner'
import { reasoningHeadline } from '@/features/agent/activity/lib/reasoning-headline'
import { useTextClock } from '@/features/agent/activity/lib/text-clock'
import { formatElapsed } from '@/features/agent/activity/lib/shelf-fit'
import { VERB_ROTATION_MS, verbAt } from '@/features/agent/activity/lib/verbs'

export interface WorkingLineProps {
  working: boolean
  /** When the turn started, or when Enter was pressed before it did. */
  since?: string
  /** A compaction opens no turn, so it shows here whether or not one is running. */
  compacting?: boolean
  /** An ask is waiting on the person. */
  asking?: boolean
  /** Stop was pressed and the turn has not ended yet. */
  stopping?: boolean
  activeTodo?: string
  /** The live label of the running main-agent tool. */
  toolLabel?: string
  /** Subagents still running. */
  agents?: number
  /** What the agent is thinking right now. */
  reasoning?: string
  /** Answer text is streaming: it is the signal, so the line names nothing. */
  streaming?: boolean
}

const clockText = (seconds: number) => (seconds > 0 ? ` · ${formatElapsed(seconds)}` : '')

/** The most specific true thing the turn is doing, never a guess, beside its clock. */
function headingOf(p: WorkingLineProps, thought: string | undefined): string | null {
  if (p.stopping) return 'Stopping…'
  if (p.asking) return 'Waiting for your answer'
  if (p.compacting) return 'Compacting…'
  if (p.activeTodo) return p.activeTodo
  if (p.toolLabel) return p.toolLabel
  if (p.agents) return `Waiting for ${p.agents} ${p.agents === 1 ? 'agent' : 'agents'}`
  if (thought) return thought
  if (p.streaming) return null
  return ''
}

export function WorkingLine(props: WorkingLineProps) {
  const { working, compacting = false, asking = false } = props
  const [tick, setTick] = useState(0)
  const thought = props.reasoning ? reasoningHeadline(props.reasoning) : null
  const heading = headingOf(props, thought?.headline || undefined)
  const rotating = working && heading === ''
  const clockRef = useTextClock(props.since, clockText)

  useEffect(() => {
    if (!rotating) {
      setTick(0)
      return
    }
    const timer = window.setInterval(() => setTick((n) => n + 1), VERB_ROTATION_MS)
    return () => window.clearInterval(timer)
  }, [rotating])

  if (!working && !compacting && !asking) return null

  return (
    <div className="activity" data-testid="agent-activity-strip">
      <div className="hd">
        {!asking && <FlickerSpinner className="size-4" />}
        <span>
          {heading !== null && (
            // Announced on change; the clock beside it stays out of the live region,
            // and the rotating verb is announced once as "Working".
            <b className="verb" data-testid="agent-working-heading" aria-live="polite">
              {heading || (
                <>
                  <span className="sr-only">Working</span>
                  <span aria-hidden="true">{`${verbAt(tick)}…`}</span>
                </>
              )}
            </b>
          )}
          {!asking && <span className="dim" ref={clockRef} />}
        </span>
      </div>
      {thought?.body && heading === thought.headline && (
        <p className="reasoning-subtitle" data-testid="agent-reasoning">
          {thought.body}
        </p>
      )}
    </div>
  )
}
