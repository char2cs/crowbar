import { cn } from '@/utils/cn'
import { IS_MOBILE } from '@/utils/platform'

// The one column box header labels and row controls share, so they align by construction: the
// switch width varies by breakpoint (~30-38px), and 56px also fits "Installed" as the UI scales.
export const PROVIDER_COLUMN_CELL = 'flex w-14 shrink-0 items-center justify-center'

// nowrap is the graceful failure: at an extreme UI scale a label overflows its column
// symmetrically instead of wrapping, and the columns stay aligned.
const PROVIDER_COLUMN_LABEL = cn(
  PROVIDER_COLUMN_CELL,
  'ui-font ui-text-xs whitespace-nowrap text-muted-foreground',
)

// Visual legend naming the row's controls; aria-hidden since each control has its own aria-label.
// The spacer mirrors the row's flexible middle, so with matching px/gap the labels sit over them.
export function ProviderColumnHeader() {
  return (
    <div
      aria-hidden="true"
      data-testid="provider-columns-header"
      className={cn('flex items-center gap-2 pb-1', IS_MOBILE ? 'px-4' : 'px-1')}
    >
      <div className="min-w-0 flex-1" />
      <span className={PROVIDER_COLUMN_LABEL}>Installed</span>
      <span className={PROVIDER_COLUMN_LABEL}>Tools</span>
      <span className={PROVIDER_COLUMN_LABEL}>Enabled</span>
    </div>
  )
}
