import { DotsSixVertical } from '@phosphor-icons/react'
import { useSortable } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { ProviderIcon } from '@/components/ui/provider-icon'
import { Switch } from '@/components/ui/switch'
import { Label } from '@/components/ui/label'
import { cn } from '@/utils/cn'
import { IS_MOBILE } from '@/utils/platform'
import { PROVIDER_COLUMN_CELL } from './provider-columns'
import type { AgentProvider } from '@/features/agent/api/agent-providers-api'

interface SortableProviderRowProps {
  provider: AgentProvider
  onToggle: (id: string, enabled: boolean) => void
  /** Flip whether this provider's agent can use Crowbar's own tools. */
  onToggleTools: (id: string, enabled: boolean) => void
}

// Drag listeners sit on the handle alone so the switches stay clickable. Tools comes before the
// provider switch (the narrower thing to take away), and neither says "MCP": the user decides
// whether the agent can reach into Crowbar, not a transport.
export function SortableProviderRow({
  provider,
  onToggle,
  onToggleTools,
}: SortableProviderRowProps) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({
    id: provider.id,
  })

  const connectedLabel = provider.connected ? 'Installed' : 'Not installed'

  return (
    <div
      ref={setNodeRef}
      style={{ transform: CSS.Transform.toString(transform), transition }}
      className={cn(
        'flex items-center gap-2',
        IS_MOBILE ? 'px-4 py-3' : 'rounded-lg px-1 py-2 transition-colors hover:bg-muted/50',
        isDragging ? 'relative z-10 opacity-40' : 'relative',
      )}
    >
      <button
        type="button"
        aria-label={`Reorder ${provider.displayName}`}
        className="flex size-6 shrink-0 cursor-grab items-center justify-center rounded text-muted-foreground transition-colors hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none active:cursor-grabbing"
        {...attributes}
        {...listeners}
      >
        <DotsSixVertical className="size-4" weight="bold" />
      </button>

      <ProviderIcon svg={provider.icon} className="text-foreground" />

      <Label className="ui-font ui-text-sm min-w-0 flex-1 cursor-default truncate font-normal text-foreground">
        {provider.displayName}
      </Label>

      {/* THE THREE CONTROL COLUMNS. Each control sits in the shared
          PROVIDER_COLUMN_CELL box that ProviderColumnHeader labels — same string,
          so the label above a control cannot drift off it. */}
      <span className={PROVIDER_COLUMN_CELL}>
        <span
          data-testid={`provider-connected-${provider.id}`}
          data-connected={provider.connected ? 'true' : 'false'}
          aria-label={connectedLabel}
          title={connectedLabel}
          className={cn(
            'size-2 shrink-0 rounded-full',
            provider.connected ? 'bg-success' : 'border border-muted-foreground/50 bg-transparent',
          )}
        />
      </span>

      {/* Crowbar's own tools, not the provider: off, the agent still runs but cannot read the
          workspace or comment through Crowbar. The column header and intro explain it. */}
      <span className={PROVIDER_COLUMN_CELL}>
        <Switch
          data-testid={`provider-tools-toggle-${provider.id}`}
          aria-label={`Let ${provider.displayName} use Crowbar's tools`}
          title={`Let ${provider.displayName} use Crowbar's tools`}
          checked={provider.mcpEnabled}
          onChange={(checked) => onToggleTools(provider.id, checked)}
          size="sm"
        />
      </span>

      <span className={PROVIDER_COLUMN_CELL}>
        <Switch
          data-testid={`provider-toggle-${provider.id}`}
          aria-label={`Enable ${provider.displayName}`}
          title={`Offer ${provider.displayName} when starting a chat`}
          checked={provider.enabled}
          onChange={(checked) => onToggle(provider.id, checked)}
          size="sm"
        />
      </span>
    </div>
  )
}
