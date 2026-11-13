import { useState } from 'react'
import { CaretDownIcon, CaretUpIcon } from '@phosphor-icons/react'
import { summarizeRows, windowRows } from '@/features/agent/trays/lib/tray-rows'
import { TrayRowView } from '@/features/agent/trays/tray-row'
import { useWorkRows } from '@/features/agent/trays/use-work-rows'
import { IS_MOBILE } from '@/utils/platform'

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
  // A phone folds two or more items into one summary row that opens to the rows.
  const grouped = IS_MOBILE && rows.length > 1
  const view = windowRows(rows, expanded || grouped)
  const summary = summarizeRows(rows)
  const folded = view.more
    ? `${view.more.hidden} more${view.more.running > 0 ? ` · ${view.more.running} running` : ''}`
    : 'Show fewer'
  return (
    <div className="tray-bottom" data-testid="agent-bottom-tray">
      <ul className="tray-list">
        {grouped && (
          <li className="tray-more tray-summary">
            <button
              type="button"
              aria-expanded={expanded}
              aria-label={summary}
              onClick={() => setExpanded((open) => !open)}
            >
              <span className="tray-summary-counts">{summary}</span>
              {expanded ? <CaretUpIcon size={12} /> : <CaretDownIcon size={12} />}
            </button>
          </li>
        )}
        {(!grouped || expanded) &&
          view.entries.map((entry) =>
            entry.type === 'label' ? (
              <li key={entry.id} className="tray-group" role="presentation">
                {entry.text}
              </li>
            ) : (
              <TrayRowView key={entry.row.id} row={entry.row} onStop={onStopShell} />
            ),
          )}
        {!grouped && (view.more || view.canCollapse) && (
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
