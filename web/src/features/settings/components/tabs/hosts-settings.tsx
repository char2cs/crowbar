import { Plus, Trash } from '@phosphor-icons/react'
import { useCallback, useState } from 'react'
import { Button } from '@/components/ui/button'
import { primitiveConfirm } from '@/components/ui/primitive-dialog-service'
import { AddHostDialog } from '@/features/hosts/components/add-host-dialog'
import { HostStatusDot } from '@/features/hosts/components/host-status-dot'
import { useHostDotState } from '@/features/hosts/hooks/use-host-status'
import { hostFailureKind } from '@/features/hosts/lib/classify-host-failure'
import { actionErrorMessage } from '@/features/hosts/lib/host-failure-notice'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { LOCAL_HOST, type Host } from '@/features/hosts/types'
import { forgetHostToken, unregisterRemoteHost } from '@/lib/crowbar-bridge'
import { apiFetch } from '@/lib/api'
import { toast } from '@/features/window/stores/toast-store'

/**
 * Revoke order matters: the daemon's own `DELETE /v0/clients/:id`, dialed
 * over THAT host's own authed channel (hostId — never 'local'), goes before
 * any local cleanup. Getting this backwards would drop the only credential
 * able to ever revoke that client, leaving it authorized on the daemon forever.
 */
async function revokeHost(host: Host): Promise<void> {
  try {
    await apiFetch(`/v0/clients/${host.clientId}`, { method: 'DELETE' }, undefined, host.id)
  } catch (err) {
    if (!(await mayRemoveUnrevoked(host, err))) return
  }
  await removeHere(host)
}

// A host that refuses this client already holds nothing it can use. One that cannot be asked
// is the user's call: removing it here leaves its pairing valid there until revoked there.
async function mayRemoveUnrevoked(host: Host, err: unknown): Promise<boolean> {
  const kind = hostFailureKind(err)
  if (kind === 'refused') {
    toast.info(
      `Removed ${host.name}`,
      'Its pairing was already revoked or lost, so there was nothing left to revoke on it.',
    )
    return true
  }
  if (kind === 'unreachable') {
    return primitiveConfirm(
      `Crowbar couldn’t reach ${host.name} to revoke this computer’s access. Remove it from this app anyway? Its pairing stays valid on ${host.name} until it is revoked there.`,
      { title: `Can’t reach ${host.name}`, confirmLabel: 'Remove anyway', cancelLabel: 'Keep it' },
    )
  }
  if (kind === 'identity-changed') {
    return primitiveConfirm(
      `The host at ${host.endpoint} no longer presents the certificate you paired with, so Crowbar won’t send it anything — not even a revoke. Remove it from this app anyway? If the original host still exists, this computer’s pairing stays valid there until it is revoked there.`,
      {
        title: `${host.name}’s identity changed`,
        confirmLabel: 'Remove anyway',
        cancelLabel: 'Keep it',
      },
    )
  }
  toast.error(
    `Could not revoke ${host.name}`,
    actionErrorMessage(err, 'The host could not be reached.'),
  )
  return false
}

// The row goes whatever the cleanup says: a live row for a host that is revoked or given up on
// is wrong. What could not be cleared is said, and logged for a bug report.
async function removeHere(host: Host): Promise<void> {
  const cleanup = await Promise.allSettled([
    unregisterRemoteHost(host.id),
    forgetHostToken(host.id),
  ])
  const failures = cleanup.flatMap((step) => (step.status === 'rejected' ? [step.reason] : []))
  if (failures.length > 0) {
    console.error(`removing ${host.name} left local state behind:`, ...failures)
    const [first] = failures
    toast.error(
      `Removed ${host.name}, but couldn’t clear everything this app kept for it`,
      first instanceof Error ? first.message : String(first),
    )
  }
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
  const state = useHostDotState(host.id)

  return (
    <div
      data-testid="host-row"
      className="flex items-center gap-3 rounded-lg px-2 py-2 transition-colors hover:bg-muted/50"
    >
      <HostStatusDot hostId={host.id} state={state} />
      <div className="min-w-0 flex-1">
        <div className="ui-font ui-text-sm truncate text-foreground">{host.name}</div>
        <div className="ui-font ui-text-sm truncate text-muted-foreground">
          {isLocal ? 'This computer' : host.endpoint}
          {state === 'no-live-stream' && ' · No live stream'}
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
  // Each opening is a new attempt, even one made while the last is still closing.
  const [addAttempt, setAddAttempt] = useState(0)
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

      <Button
        type="button"
        variant="outline"
        onClick={() => {
          setAddAttempt((n) => n + 1)
          setAddOpen(true)
        }}
      >
        <Plus />
        Add host
      </Button>

      <AddHostDialog key={addAttempt} open={addOpen} onOpenChange={setAddOpen} />
    </div>
  )
}
