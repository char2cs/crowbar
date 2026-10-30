import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { HostStatusIcon, hostPlace } from '@/features/hosts/components/host-status-icon'
import { useHostView } from '@/features/hosts/hooks/use-host-status'
import { hostDotLabel } from '@/features/hosts/lib/host-tone'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { LOCAL_HOST } from '@/features/hosts/types'
import { cn } from '@/lib/utils'

interface SidebarHostPickerProps {
  hostIds: readonly string[]
  /** The host whose projects the switcher lists. */
  shown: string
  onSelect: (hostId: string) => void
  /** Sizes the trigger: the phone's page needs a touch-sized one. */
  className?: string
}

function HostRow({
  hostId,
  current,
  onSelect,
}: {
  hostId: string
  current: boolean
  onSelect: (hostId: string) => void
}) {
  const { name, state } = useHostView(hostId)
  const endpoint = useHostConfigStore((s) => s.hosts.find((h) => h.id === hostId)?.endpoint)
  const where = hostId === LOCAL_HOST.id ? 'This machine' : endpoint
  return (
    <DropdownMenuItem onClick={() => onSelect(hostId)} aria-current={current || undefined}>
      <HostStatusIcon hostId={hostId} state={state} />
      <span className="flex min-w-0 flex-1 flex-col">
        <span className="truncate">{name ?? hostId}</span>
        {where && <span className="truncate text-xs text-muted-foreground">{where}</span>}
      </span>
      {current && <span className="shrink-0 text-xs text-muted-foreground">Current</span>}
    </DropdownMenuItem>
  )
}

/**
 * The host whose projects the switcher lists: its computer (this machine) or cloud (remote) wearing the status dot, and its name, opening the list of hosts.
 * A view choice only, so the hosts not shown stay connected.
 */
export function SidebarHostPicker({ hostIds, shown, onSelect, className }: SidebarHostPickerProps) {
  const { name, state } = useHostView(shown)
  const label = name ?? shown
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={<button type="button" />}
        aria-label={`Host: ${label}`}
        title={`${hostPlace(shown) === 'local' ? 'Local' : 'Remote'} · ${hostDotLabel(state)} · Switch host`}
        data-testid="sidebar-host-picker"
        className={cn(
          'inline-flex h-6 min-w-0 items-center gap-1.5 rounded-full px-2 text-xs font-medium text-muted-foreground hover:bg-sidebar-element-hover hover:text-foreground',
          className,
        )}
      >
        <HostStatusIcon hostId={shown} state={state} showTitle={false} />
        <span className="max-w-24 truncate">{label}</span>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="center" className="w-max min-w-56">
        {hostIds.map((id) => (
          <HostRow key={id} hostId={id} current={id === shown} onSelect={onSelect} />
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
