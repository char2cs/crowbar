import {
  FOLD_AWAY_GLYPH_BAR,
  FOLD_AWAY_GLYPH_CHEVRON,
  ROW_SUB_ACTION_FOLD_AWAY,
} from './workspace-row-base'

/**
 * The control on a row that is folded but still holding others: it lets them go.
 *
 * Present only while there is something to release, and out of the flow until
 * the row is hovered or focused — the same rule the add-child "+" follows, and
 * for the same reason: a hidden-but-present button still costs the row its 24px
 * box and its 6px gap. It fades as it arrives; see ROW_SUB_ACTION_FOLD_AWAY.
 *
 * This is the only thing that dismisses a keep set wholesale. Removing one row
 * from it is cmd-clicking that row off.
 */
export function FoldAwayButton({ label, onFold }: { label: string; onFold: () => void }) {
  return (
    <button
      type="button"
      className={ROW_SUB_ACTION_FOLD_AWAY}
      aria-label={`Fold away the rows ${label} is holding`}
      title="Fold these away"
      onClick={(e) => {
        e.stopPropagation()
        onFold()
      }}
      onPointerDown={(e) => e.stopPropagation()}
    >
      <svg
        aria-hidden="true"
        className="size-3"
        viewBox="0 0 16 16"
        fill="none"
        stroke="currentColor"
        strokeWidth="1.6"
        strokeLinecap="round"
        strokeLinejoin="round"
      >
        <path d={FOLD_AWAY_GLYPH_BAR} />
        <path d={FOLD_AWAY_GLYPH_CHEVRON} />
      </svg>
    </button>
  )
}
