import { useState } from 'react'
import { CaretDownIcon, CaretUpIcon } from '@phosphor-icons/react'
import { FlickerSpinner } from '@/components/ui/flicker-spinner'
import type { PlanItem } from '@/features/agent/api/chat-rows'

/** One pip per step up to this many; longer plans scale their progress onto it. */
const MAX_PIPS = 8

/** The agent's to-do list behind the input box's top edge: progress and the current step
 *  collapsed, every step once opened. Absent once nothing is left to do. */
export function PlanTray({ plan }: { plan: PlanItem[] | undefined }) {
  const [open, setOpen] = useState(false)
  const steps = plan ?? []
  const done = steps.filter((step) => step.status === 'done').length
  if (steps.length === 0 || done === steps.length) return null
  const pips = Math.min(steps.length, MAX_PIPS)
  const current =
    steps.find((step) => step.status === 'active') ?? steps.find((step) => step.status !== 'done')
  return (
    <section className="tray-section" data-testid="agent-todo-tray">
      <button
        type="button"
        className="tray-head"
        aria-expanded={open}
        onClick={() => setOpen((o) => !o)}
      >
        <span className="tray-pips" aria-hidden="true">
          {[...Array(pips).keys()].map((pip) => (
            <i key={pip} data-on={pip < Math.round((done / steps.length) * pips) || undefined} />
          ))}
        </span>
        <b>
          {done} of {steps.length} done
        </b>
        <span className="tray-current">{open ? '' : current?.text}</span>
        {open ? <CaretUpIcon size={12} /> : <CaretDownIcon size={12} />}
      </button>
      {open && (
        <ol className="tray-list">
          {steps.map((step, i) => (
            // react-doctor-disable-next-line no-array-index-as-key -- a replaced plan's steps carry no id; the text is part of the key
            <li key={step.id ?? `${i}-${step.text}`} data-status={step.status}>
              {step.status === 'active' ? (
                <FlickerSpinner className="size-4 text-primary" />
              ) : (
                <i className="tray-gap" />
              )}
              <span>{step.text}</span>
            </li>
          ))}
        </ol>
      )}
    </section>
  )
}
