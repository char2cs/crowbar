import { useEffect, useState } from 'react'
import { FlickerSpinner } from '@/components/ui/flicker-spinner'
import { reasoningHeadline } from '@/features/agent/activity/lib/reasoning-headline'
import { formatClock, useTextClock } from '@/features/agent/activity/lib/text-clock'
import { VERB_ROTATION_MS, verbAt } from '@/features/agent/activity/lib/verbs'

export interface WorkingLineProps {
  working: boolean
  since?: string
  compacting?: boolean
  asking?: boolean
  stopping?: boolean
  activeTodo?: string
  toolLabel?: string
  agents?: number
  reasoning?: string
  streaming?: boolean
  connecting?: string
}

const clockText = (seconds: number) => (seconds > 0 ? ` · ${formatClock(seconds)}` : '')

function headingOf(p: WorkingLineProps, thought: string | undefined): string {
  if (p.stopping) return 'Stopping…'
  if (p.connecting && !p.working) return p.connecting
  return thought ?? ''
}

const shownElsewhere = (p: WorkingLineProps) =>
  !!(p.asking || p.compacting || p.toolLabel || p.activeTodo || p.streaming)

export function WorkingLine(props: WorkingLineProps) {
  const { working, connecting } = props
  const [tick, setTick] = useState(0)
  const thought = props.reasoning ? reasoningHeadline(props.reasoning) : null
  const quiet = shownElsewhere(props) && !props.stopping
  const heading = headingOf(props, thought?.headline || undefined)
  const onlyAgents = working && !!props.agents && heading === ''
  const rotating = working && heading === '' && !onlyAgents && !quiet
  const clockRef = useTextClock(props.since, clockText)

  useEffect(() => {
    if (!rotating) {
      setTick(0)
      return
    }
    const timer = window.setInterval(() => setTick((n) => n + 1), VERB_ROTATION_MS)
    return () => window.clearInterval(timer)
  }, [rotating])

  if (quiet || onlyAgents || (!working && !connecting)) return null

  return (
    <div className="activity" data-testid="agent-activity-strip">
      <div className="hd">
        <FlickerSpinner className="size-4" />
        <span>
          <b className="verb" data-testid="agent-working-heading" aria-live="polite">
            {heading || (
              <>
                <span className="sr-only">Working</span>
                <span aria-hidden="true">{`${verbAt(tick)}…`}</span>
              </>
            )}
          </b>
          <span className="dim" ref={clockRef} />
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
