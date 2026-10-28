import { CaretDownIcon, StackIcon } from '@phosphor-icons/react'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { HostStatusIcon } from '@/features/hosts/components/host-status-icon'
import { useHostView } from '@/features/hosts/hooks/use-host-status'
import { ALL_HOSTS } from '@/features/hosts/lib/host-filter'
import { hostDotLabel, hostToneText } from '@/features/hosts/lib/host-tone'
import { cn } from '@/lib/utils'

interface SidebarHostPickerProps {
  /** Every host the list can be narrowed to; with fewer than two there is nothing to choose. */
  hostIds: readonly string[]
  /** ALL_HOSTS or the host whose projects the switcher lists. */
  shown: string
  onSelect: (hostId: string) => void
  /** Sizes the trigger: the phone's page needs a touch-sized one. */
  className?: string
}

function HostRow({ hostId }: { hostId: string }) {
  const { name, state } = useHostView(hostId)
  return (
    <DropdownMenuRadioItem value={hostId} closeOnClick>
      <HostStatusIcon hostId={hostId} state={state} />
      <span className="min-w-0 flex-1 truncate">{name ?? hostId}</span>
      {/* The dot's words and tone, as Settings and Create Space say them. */}
      <span {...hostToneText(state, 'text-xs')}>{hostDotLabel(state)}</span>
    </DropdownMenuRadioItem>
  )
}

/**
 * Chooses which host's projects the project switcher lists: "All hosts", or one host with its
 * status dot. A view filter, so the hosts left out stay connected. Absent with a single host.
 */
export function SidebarHostPicker({ hostIds, shown, onSelect, className }: SidebarHostPickerProps) {
  const { name, state } = useHostView(shown)
  if (hostIds.length < 2) return null

  const label = shown === ALL_HOSTS ? 'All hosts' : (name ?? shown)
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={<Button variant="ghost" size="sm" />}
        aria-label={`Host: ${label}`}
        data-testid="sidebar-host-picker"
        className={cn(
          'min-w-0 max-w-full gap-1.5 rounded-sm px-2 text-xs text-muted-foreground hover:bg-sidebar-element-hover',
          className,
        )}
      >
        {shown === ALL_HOSTS ? (
          <StackIcon aria-hidden="true" className="size-4 shrink-0" />
        ) : (
          <HostStatusIcon hostId={shown} state={state} showTitle={false} className="shrink-0" />
        )}
        <span className="truncate">{label}</span>
        <CaretDownIcon aria-hidden="true" className="size-3 shrink-0" />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="center" className="w-max min-w-48">
        <DropdownMenuRadioGroup value={shown} onValueChange={(hostId) => onSelect(hostId)}>
          <DropdownMenuRadioItem value={ALL_HOSTS} closeOnClick>
            <StackIcon aria-hidden="true" className="size-4" />
            <span className="flex-1">All hosts</span>
          </DropdownMenuRadioItem>
          <DropdownMenuSeparator />
          {hostIds.map((id) => (
            <HostRow key={id} hostId={id} />
          ))}
        </DropdownMenuRadioGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
