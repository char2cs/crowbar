import { Plus, Trash } from '@phosphor-icons/react'
import { useCallback, useState } from 'react'
import { Button } from '@/components/ui/button'
import { AddHostDialog } from '@/features/hosts/components/add-host-dialog'
import { useHostStatus } from '@/features/hosts/hooks/use-host-status'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { LOCAL_HOST, type Host, type HostStatus } from '@/features/hosts/types'
import { forgetHostToken, unregisterRemoteHost } from '@/lib/crowbar-bridge'
import { apiFetch } from '@/lib/api'
import { toast } from '@/features/window/stores/toast-store'
import { cn } from '@/utils/cn'

const STATUS_LABEL: Record<HostStatus, string> = {
  unknown: 'Status unknown',
  probing: 'Checking…',
  up: 'Online',
  down: 'Offline',
  'needs-repair': 'Needs repair — reconnect this host',
}

const STATUS_DOT_CLASS: Record<HostStatus, string> = {
  unknown: 'border border-muted-foreground/50 bg-transparent',
  probing: 'border border-muted-foreground/50 bg-transparent',
  up: 'bg-success',
  down: 'bg-destructive',
  'needs-repair': 'bg-warning',
}

/** A row's live dot: the status the app's own prober last recorded for the host. */
function HostStatusDot({ hostId }: { hostId: string }) {
  const status = useHostStatus(hostId)

  return (
    <span
      data-testid={`host-status-${hostId}`}
      aria-label={STATUS_LABEL[status]}
      title={STATUS_LABEL[status]}
      className={cn('size-2 shrink-0 rounded-full', STATUS_DOT_CLASS[status])}
    />
  )
}

/**
 * Revoke order matters: the daemon's own `DELETE /v0/clients/:id`, dialed
 * over THAT host's own authed channel (hostId — never 'local'), must
 * succeed before any local cleanup. Getting this backwards would drop the
 * only credential able to ever revoke that client, leaving it authorized on
 * the daemon forever. A failure here is surfaced and stops immediately —
 * the local token/config row is left exactly as it was.
 */
async function revokeHost(host: Host): Promise<void> {
  try {
    await apiFetch(`/v0/clients/${host.clientId}`, { method: 'DELETE' }, undefined, host.id)
  } catch (err) {
    toast.error(
      `Could not revoke ${host.name}`,
      err instanceof Error ? err.message : 'The host could not be reached.',
    )
    return
  }
  await unregisterRemoteHost(host.id)
  await forgetHostToken(host.id)
  // Its rows, status, streams and provider list follow the removal (onHostRemoved).
  useHostConfigStore.getState().removeHost(host.id)
}

interface HostRowProps {
  host: Host
  isLocal: boolean
  revoking: boolean
  onRevoke: (host: Host) => void
}

function HostRow({ host, isLocal, revoking, onRevoke }: HostRowProps) {
  return (
    <div
      data-testid="host-row"
      className="flex items-center gap-3 rounded-lg px-2 py-2 transition-colors hover:bg-muted/50"
    >
      <HostStatusDot hostId={host.id} />
      <div className="min-w-0 flex-1">
        <div className="ui-font ui-text-sm truncate text-foreground">{host.name}</div>
        <div className="ui-font ui-text-sm truncate text-muted-foreground">
          {isLocal ? 'This computer' : host.endpoint}
        </div>
      </div>
      {!isLocal && (
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          disabled={revoking}
          aria-label={`Revoke ${host.name}`}
          title={`Revoke ${host.name}`}
          className="text-destructive hover:bg-destructive/10"
          onClick={() => onRevoke(host)}
        >
          <Trash />
        </Button>
      )}
    </div>
  )
}

/**
 * Settings → Hosts: Local plus every paired remote daemon, each with a live
 * status dot, an Add host flow, and per-host revoke. Desktop-only — gated
 * out of SETTINGS_TAB_ITEMS in a non-Tauri build.
 */
export function HostsSettings() {
  const hosts = useHostConfigStore((s) => s.hosts)
  const [addOpen, setAddOpen] = useState(false)
  // Which host's revoke is in flight, so a second click can't fire a second
  // DELETE against the same client while the first is still out.
  const [revokingId, setRevokingId] = useState<string | null>(null)

  const handleRevoke = useCallback((host: Host) => {
    setRevokingId(host.id)
    void revokeHost(host).finally(() => {
      setRevokingId((current) => (current === host.id ? null : current))
    })
  }, [])

  return (
    <div className="space-y-4">
      <div className="mb-2 px-1 py-1.5">
        <h3 className="font-heading text-[32px] text-foreground">Hosts</h3>
        <p className="ui-font ui-text-sm text-muted-foreground">
          Crowbar always runs on this computer. Add another machine running its own daemon to reach
          it from here too.
        </p>
      </div>

      <div className="space-y-1">
        <HostRow host={LOCAL_HOST} isLocal revoking={false} onRevoke={() => {}} />
        {hosts.map((host) => (
          <HostRow
            key={host.id}
            host={host}
            isLocal={false}
            revoking={revokingId === host.id}
            onRevoke={handleRevoke}
          />
        ))}
      </div>

      <Button type="button" variant="outline" onClick={() => setAddOpen(true)}>
        <Plus />
        Add host
      </Button>

      <AddHostDialog open={addOpen} onOpenChange={setAddOpen} />
    </div>
  )
}
