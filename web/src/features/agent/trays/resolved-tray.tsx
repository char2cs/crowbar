import { useState } from 'react'
import { CaretDownIcon, CaretUpIcon } from '@phosphor-icons/react'
import type { AgentChoice } from '@/features/agent/api/agent-chat-activity'
import { describeResolvedChoice } from '@/features/agent/composer/lib/ask-labels'

function verdict(choice: AgentChoice): 'allowed' | 'denied' | null {
  const picked = new Set(choice.answeredOptionIds)
  let allowed = false
  for (const option of choice.options) {
    if (!picked.has(option.id)) continue
    if (option.kind === 'deny') return 'denied'
    allowed ||= option.kind === 'allow'
  }
  return allowed ? 'allowed' : null
}

/** "2 allowed · 1 denied"; empty when the decisions were not allow-or-deny. */
function split(choices: AgentChoice[]): string {
  const counts = { allowed: 0, denied: 0 }
  for (const choice of choices) {
    const kind = verdict(choice)
    if (kind) counts[kind] += 1
  }
  return (['allowed', 'denied'] as const)
    .filter((kind) => counts[kind] > 0)
    .map((kind) => `${counts[kind]} ${kind}`)
    .join(' · ')
}

/** The requests answered since the last message, folded into one line that opens to one row each. */
export function ResolvedTray({ choices }: { choices: AgentChoice[] }) {
  const [open, setOpen] = useState(false)
  if (choices.length === 0) return null
  const summary = split(choices)
  return (
    <section className="tray-section" data-testid="agent-resolved-tray">
      <button
        type="button"
        className="tray-head"
        aria-expanded={open}
        onClick={() => setOpen((o) => !o)}
      >
        <b>
          {choices.length} {choices.length === 1 ? 'request' : 'requests'} answered
        </b>
        <span className="tray-current">{summary}</span>
        {open ? <CaretUpIcon size={12} /> : <CaretDownIcon size={12} />}
      </button>
      {open && (
        <ul className="tray-list">
          {choices.map((choice) => (
            <li key={choice.id}>
              <span>{describeResolvedChoice(choice)}</span>
            </li>
          ))}
        </ul>
      )}
    </section>
  )
}
