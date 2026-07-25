import { DotsSixVertical } from '@phosphor-icons/react'
import { useSortable } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { ProviderIcon } from '@/features/agent/components/provider-icon'
import { Switch } from '@/components/ui/switch'
import { Label } from '@/components/ui/label'
import { cn } from '@/utils/cn'
import type { AgentProvider } from '@/features/agent/api/agent-api'

interface SortableProviderRowProps {
  provider: AgentProvider
  onToggle: (id: string, enabled: boolean) => void
}

/**
 * One provider row in the Providers settings tab: a drag handle for priority,
 * the provider glyph + name, a connected (installed) indicator, and an enable
 * toggle. Extracted from the tab so the tab stays within react-doctor's
 * `no-giant-component`; the `useSortable` wiring mirrors `sortable-editor-tab.tsx`
 * but puts the drag listeners on the handle alone so the Switch stays clickable.
 */
export function SortableProviderRow({ provider, onToggle }: SortableProviderRowProps) {
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

      <Switch
        data-testid={`provider-toggle-${provider.id}`}
        aria-label={`Enable ${provider.displayName}`}
        checked={provider.enabled}
        onChange={(checked) => onToggle(provider.id, checked)}
        size="sm"
      />
    </div>
  )
}
