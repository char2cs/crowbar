import { useState } from 'react'
import { CaretDownIcon, CaretUpIcon } from '@phosphor-icons/react'
import { windowRows } from '@/features/agent/trays/lib/tray-rows'
import { TrayRowView } from '@/features/agent/trays/tray-row'
import { useWorkRows } from '@/features/agent/trays/use-work-rows'

/** Subagents (and shells) behind the input box's bottom edge: at most four rows, the rest
 *  gathered into one row that opens. Absent while nothing runs. */
export function BottomTray({
  chatId,
  onStopShell,
}: {
  chatId: string
  onStopShell?: (shellId: string) => void
}) {
  const rows = useWorkRows(chatId)
  const [expanded, setExpanded] = useState(false)
  if (rows.length === 0) return null
  const view = windowRows(rows, expanded)
  const folded = view.more
    ? `${view.more.hidden} more${view.more.running > 0 ? ` · ${view.more.running} running` : ''}`
    : 'Show fewer'
  return (
    <div className="tray-bottom" data-testid="agent-bottom-tray">
      <ul className="tray-list">
        {view.entries.map((entry) =>
          entry.type === 'label' ? (
            <li key={entry.id} className="tray-group" role="presentation">
              {entry.text}
            </li>
          ) : (
            <TrayRowView key={entry.row.id} row={entry.row} onStop={onStopShell} />
          ),
        )}
        {(view.more || view.canCollapse) && (
          <li className="tray-more">
            <button
              type="button"
              aria-expanded={expanded}
              onClick={() => setExpanded((open) => !open)}
            >
              {view.more ? <CaretDownIcon size={12} /> : <CaretUpIcon size={12} />}
              <span>{folded}</span>
            </button>
          </li>
        )}
      </ul>
    </div>
  )
}
