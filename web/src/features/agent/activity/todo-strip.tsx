import { useState } from 'react'
import { CaretRightIcon } from '@phosphor-icons/react'
import type { PlanItem } from '@/features/agent/api/chat-rows'

/** The main agent's plan, docked above the composer: the active step and the
 *  progress, every step once opened. Gone once nothing is left to do. */
export function TodoStrip({ plan }: { plan: PlanItem[] | undefined }) {
  const [open, setOpen] = useState(false)
  const steps = plan ?? []
  const done = steps.filter((step) => step.status === 'done').length
  if (steps.length === 0 || done === steps.length) return null
  const active =
    steps.find((step) => step.status === 'active') ?? steps.find((s) => s.status !== 'done')
  return (
    <div className="todo-strip" data-testid="agent-todo-strip">
      <button
        type="button"
        className="todo-strip-head"
        aria-expanded={open}
        onClick={() => setOpen((o) => !o)}
      >
        <span className="todo-strip-active">{active?.text}</span>
        <span className="todo-strip-count">
          {done}/{steps.length}
        </span>
        <CaretRightIcon size={12} />
      </button>
      {open && (
        <ol className="plan">
          {steps.map((step, i) => (
            // react-doctor-disable-next-line no-array-index-as-key -- a replaced plan's steps carry no id; the text is part of the key
            <li key={step.id ?? `${i}-${step.text}`} data-status={step.status}>
              {step.text}
            </li>
          ))}
        </ol>
      )}
    </div>
  )
}
