import { useState } from 'react'
import { CaretDownIcon, CaretUpIcon } from '@phosphor-icons/react'
import type { AgentChoice } from '@/features/agent/api/agent-chat-activity'
import { describeChoice, optionLabel } from '@/features/agent/composer/lib/ask-labels'

type Outcome = 'allowed' | 'denied' | 'timed out' | 'not answered'

interface Verdict {
  outcome: Outcome
  /** What the row says: the option the person picked, in the past tense. */
  words: string
}

function pickedOption(choice: AgentChoice) {
  const picked = new Set(choice.answeredOptionIds)
  return choice.options.find((option) => picked.has(option.id))
}

function verdict(choice: AgentChoice): Verdict | null {
  if (choice.resolution === 'proceeded') return { outcome: 'allowed', words: 'Allowed in terminal' }
  if (choice.resolution === 'refused') return { outcome: 'timed out', words: 'Timed out' }
  if (choice.resolution === 'abandoned') return { outcome: 'not answered', words: 'Not answered' }
  const option = pickedOption(choice)
  if (!option) return null
  if (option.kind === 'deny') return { outcome: 'denied', words: 'Denied' }
  if (option.kind !== 'allow') return null
  const label = optionLabel(option)
  return { outcome: 'allowed', words: label.replace(/^Allow\b/, 'Allowed') }
}

const ORDER: Outcome[] = ['allowed', 'denied', 'timed out', 'not answered']

/** "2 allowed · 1 denied"; empty when the decisions were not allow-or-deny. */
function split(verdicts: (Verdict | null)[]): string {
  return ORDER.map(
    (outcome) => [outcome, verdicts.filter((v) => v?.outcome === outcome).length] as const,
  )
    .filter(([, n]) => n > 0)
    .map(([outcome, n]) => `${n} ${outcome}`)
    .join(' · ')
}

const clock = (at?: string) =>
  at
    ? new Date(at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', hour12: false })
    : ''

/** The requests settled since the last message, folded into one line that opens to one row each. */
export function ResolvedTray({ choices }: { choices: AgentChoice[] }) {
  const [open, setOpen] = useState(false)
  if (choices.length === 0) return null
  const verdicts = choices.map(verdict)
  const unanswered = verdicts.some(
    (v) => v?.outcome === 'timed out' || v?.outcome === 'not answered',
  )
  const noun = choices.length === 1 ? 'request' : 'requests'
  return (
    <section className="tray-section rq-hist" data-testid="agent-resolved-tray">
      <button
        type="button"
        className="rq-hsum"
        aria-expanded={open}
        onClick={() => setOpen((o) => !o)}
      >
        <span className="rq-hsum-text">
          <b>
            {choices.length} {noun} {unanswered ? 'closed' : 'answered'}
          </b>
          <span className="rq-hsum-split">{split(verdicts)}</span>
        </span>
        <span className="rq-hsum-toggle">
          {open ? 'Hide' : 'Show'}
          {open ? <CaretUpIcon size={12} /> : <CaretDownIcon size={12} />}
        </span>
      </button>
      {open && (
        <ul className="rq-hrows">
          {choices.map((choice, i) => {
            const v = verdicts[i]
            return (
              <li key={choice.id} className="rq-hrow">
                <span className="rq-hverdict" data-outcome={v?.outcome}>
                  {v?.words ?? 'Answered'}
                </span>
                <span className="rq-hwhat">{choice.toolName || describeChoice(choice)}</span>
                <span className="rq-htime">{clock(choice.resolvedAt)}</span>
              </li>
            )
          })}
        </ul>
      )}
    </section>
  )
}
