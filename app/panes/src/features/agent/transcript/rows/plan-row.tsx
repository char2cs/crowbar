import { memo, useState } from 'react'
import { CaretRightIcon } from '@phosphor-icons/react'
import type { PlanSnapshotItem } from '@/features/agent/stores/lib/transcript-items'

/** The plan as its turn left it, collapsed to its progress. */
function PlanRowBody({ item }: { item: PlanSnapshotItem }) {
  const [open, setOpen] = useState(false)
  const done = item.items.filter((step) => step.status === 'done').length
  return (
    <article className="row plan-row" data-row-id={item.id}>
      <div className="assistant">
        <button
          type="button"
          className="reasoning-toggle"
          aria-expanded={open}
          onClick={() => setOpen((o) => !o)}
        >
          Tasks · {done}/{item.items.length} done
          <CaretRightIcon size={12} />
        </button>
        {open && (
          <ol className="plan">
            {item.items.map((step, i) => (
              // react-doctor-disable-next-line no-array-index-as-key -- a replaced plan's steps carry no id; the text is part of the key
              <li key={step.id ?? `${i}-${step.text}`} data-status={step.status}>
                {step.text}
              </li>
            ))}
          </ol>
        )}
      </div>
    </article>
  )
}

export const PlanRow = memo(PlanRowBody)
