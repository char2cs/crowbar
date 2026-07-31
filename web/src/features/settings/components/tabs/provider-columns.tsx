import { cn } from '@/utils/cn'

/**
 * THE ONE COLUMN DEFINITION the Providers table has. The header labels and the
 * row controls both render this exact string, so they cannot drift: there is no
 * second width to keep in sync, and a change here moves both at once.
 *
 * It is a fixed-width centering box rather than a hugging one because the things
 * it centres are NOT the same width as each other, and two of them are not even
 * a stable width: `Switch` is `w-[calc(var(--thumb-size)*2-2px)]` with
 * `--thumb-size` at `--spacing(5)` narrow and `--spacing(4)` from `sm:` up
 * (components/ui/switch.tsx) — ~38px on a narrow settings pane, ~30px on a wide
 * one. A header label sized to today's switch would sit off-centre at the other
 * breakpoint. Centring both inside a box wider than the widest switch makes the
 * column line up by construction, at any viewport.
 *
 * WHY 56px AND NOT 40. 40px clears the 38px switch, but the header labels are
 * text at `--ui-text-xs` — `calc(0.6875rem * var(--app-ui-scale))`, which the
 * user scales from Appearance. "Installed" already runs wider than 40px at scale
 * 1, so a 40px column would wrap it or push it into its neighbour the moment the
 * UI is scaled up. 56px holds the longest label with room to grow, and still
 * leaves the provider name (`flex-1`, truncating) the rest of the row.
 */
export const PROVIDER_COLUMN_CELL = 'flex w-14 shrink-0 items-center justify-center'

/** Header labels: the row's own muted `ui-font ui-text-xs` register, one line
 *  each. `whitespace-nowrap` is the graceful failure — at an extreme UI scale a
 *  label overflows its column symmetrically (justify-center) instead of wrapping
 *  to two lines, and the columns stay aligned either way. */
const PROVIDER_COLUMN_LABEL = cn(
  PROVIDER_COLUMN_CELL,
  'ui-font ui-text-xs whitespace-nowrap text-muted-foreground',
)

/**
 * The table header over the provider list: what each control in the row is.
 *
 * Two unlabelled switches side by side is a guessing game, and the row could
 * only ever afford one inline word ("Tools") — which then had to be repeated on
 * every row and still left the other switch anonymous. Naming the columns once,
 * above, retires both problems.
 *
 * PRESENTATIONAL ONLY. Every control underneath already carries its own
 * `aria-label` ("Let X use Crowbar's tools", "Enable X", "Installed" /
 * "Not installed"), so a screen reader that also read this strip would announce
 * each column twice — hence `aria-hidden`. It is a visual legend, not a label.
 *
 * The leading spacer mirrors the row's flexible middle (drag handle + glyph +
 * name), so with the same `px-1` and `gap-2` as the row, and the same
 * fixed-width cells, the labels land exactly over their controls.
 */
export function ProviderColumnHeader() {
  return (
    <div
      aria-hidden="true"
      data-testid="provider-columns-header"
      className="flex items-center gap-2 px-1 pb-1"
    >
      <div className="min-w-0 flex-1" />
      <span className={PROVIDER_COLUMN_LABEL}>Installed</span>
      <span className={PROVIDER_COLUMN_LABEL}>Tools</span>
      <span className={PROVIDER_COLUMN_LABEL}>Enabled</span>
    </div>
  )
}
