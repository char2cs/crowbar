import { DotsSixVertical } from '@phosphor-icons/react'
import { useSortable } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { ProviderIcon } from '@/features/agent/components/provider-icon'
import { Switch } from '@/components/ui/switch'
import { Label } from '@/components/ui/label'
import { cn } from '@/utils/cn'
import { PROVIDER_COLUMN_CELL } from './provider-columns'
import type { AgentProvider } from '@/features/agent/api/agent-api'

interface SortableProviderRowProps {
  provider: AgentProvider
  onToggle: (id: string, enabled: boolean) => void
  /** Flip whether this provider's agent can use Crowbar's own tools. */
  onToggleTools: (id: string, enabled: boolean) => void
}

/**
 * One provider row in the Providers settings tab: a drag handle for priority,
 * the provider glyph + name, a connected (installed) indicator, a Crowbar-tools
 * toggle, and an enable toggle. Extracted from the tab so the tab stays within
 * react-doctor's `no-giant-component`; the `useSortable` wiring mirrors
 * `sortable-editor-tab.tsx` but puts the drag listeners on the handle alone so
 * the switches stay clickable.
 *
 * The two switches are independent axes, and the order they sit in says so: tools
 * first (a narrower thing you can take away), then the provider itself. Neither
 * says "MCP" anywhere a user can read — the transport is not what is being
 * decided here; whether the agent can reach into Crowbar is.
 */
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
        'flex items-center gap-2 rounded-lg px-1 py-2 transition-colors hover:bg-muted/50',
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

      {/* Crowbar's own tools, not the provider. A row with this off still opens
          chats and still runs the CLI — the agent just cannot read the workspace
          or leave review comments through Crowbar. The row no longer spells that
          out inline: the header names the column once, and the intro paragraph
          above the list carries what the one word cannot. */}
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
