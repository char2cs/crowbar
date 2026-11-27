import { cn } from '@/utils/cn'
import { IS_MOBILE } from '@/utils/platform'

export const PROVIDER_COLUMN_CELL = 'flex w-14 shrink-0 items-center justify-center'

const PROVIDER_COLUMN_LABEL = cn(
  PROVIDER_COLUMN_CELL,
  'ui-font ui-text-xs whitespace-nowrap text-muted-foreground',
)

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
