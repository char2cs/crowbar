import { Plus, Trash } from '@phosphor-icons/react'
import { useCallback, useState } from 'react'
import { Button } from '@/components/ui/button'
import { primitiveConfirm } from '@/components/ui/primitive-dialog-service'
import { AddHostDialog } from '@/features/hosts/components/add-host-dialog'
import { HostStatusIcon } from '@/features/hosts/components/host-status-icon'
import { useHostDotState } from '@/features/hosts/hooks/use-host-status'
import { hostFailureKind } from '@/features/hosts/lib/classify-host-failure'
import { actionErrorMessage, carriesOnlyStatus } from '@/features/hosts/lib/host-failure-notice'
import { hostDotLabel, hostToneText } from '@/features/hosts/lib/host-tone'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { LOCAL_HOST, type Host } from '@/features/hosts/types'
import { forgetHostToken, unregisterRemoteHost } from '@/lib/crowbar-bridge'
import { apiFetch } from '@/lib/api'
import { ApiError } from '@/lib/api/errors'
import { toast } from '@/features/window/stores/toast-store'

// The host's own DELETE (over its channel, never Local's) goes first: cleaning up here first
// would drop the only credential that can ever revoke this client there.
async function revokeHost(host: Host): Promise<void> {
  const confirmed = await primitiveConfirm(
    'To use it again you’ll need to run crowbar pair on it.',
    {
      title: `Revoke ${host.name}?`,
      confirmLabel: 'Revoke',
      cancelLabel: 'Keep it',
      destructive: true,
    },
  )
  if (!confirmed) return
  try {
    await apiFetch(`/v0/clients/${host.clientId}`, { method: 'DELETE' }, undefined, host.id)
  } catch (err) {
    if (!(await mayRemoveUnrevoked(host, err))) return
  }
  await removeHere(host)
}

// A host whose own 401 refuses this client holds nothing it can use. Any other failed revoke is
// the user's call, never a host they cannot remove: no tool can end the pairing on the host itself.
async function mayRemoveUnrevoked(host: Host, err: unknown): Promise<boolean> {
  if (refusedByHost(err)) {
    toast.info(
      `Removed ${host.name}`,
      'Its pairing was already revoked or lost, so there was nothing left to revoke on it.',
    )
    return true
  }
  const { title, why, forgets = FORGETS, listed } = unrevoked(host, err)
  const message = [why, 'Remove it from this app anyway?', forgets, listed].filter(Boolean)
  return primitiveConfirm(message.join(' '), {
    title,
    confirmLabel: 'Remove anyway',
    cancelLabel: 'Keep it',
    destructive: true,
  })
}

// The daemon refuses in its own words; the proxy's status-only 401 means this computer holds no
// key to send, so the host was never asked.
function refusedByHost(err: unknown): boolean {
  return hostFailureKind(err) === 'refused' && err instanceof ApiError && !carriesOnlyStatus(err)
}

const FORGETS = 'This computer forgets its key now, so nothing here can use it.'

interface Unrevoked {
  title: string
  why: string
  /** What removing it here does; empty when there is nothing here to forget. */
  forgets?: string
  listed: string
}

function unrevoked(host: Host, err: unknown): Unrevoked {
  const listed = `${host.name} will still list this computer as paired.`
  switch (hostFailureKind(err)) {
    case 'unreachable':
      return {
        title: `Can’t reach ${host.name}`,
        why: `Crowbar couldn’t reach ${host.name} to revoke this computer’s access.`,
        listed,
      }
    case 'identity-changed':
      return {
        title: `${host.name}’s identity changed`,
        why: `The host at ${host.endpoint} no longer presents the certificate you paired with, so Crowbar won’t send it anything — not even a revoke.`,
        listed: 'If the original host still exists, it will still list this computer as paired.',
      }
    case 'refused':
      return {
        title: `Couldn’t revoke ${host.name}`,
        why: `This computer holds no key for ${host.name}, so Crowbar couldn’t ask it to revoke this computer’s access.`,
        forgets: '',
        listed,
      }
    default:
      return {
        title: `Couldn’t revoke ${host.name}`,
        why: `The revoke failed: ${actionErrorMessage(err, String(err))}.`,
        listed,
      }
  }
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

const SECONDARY = 'ui-font ui-text-sm truncate text-muted-foreground'

function HostRow({ host, isLocal, revoking, onRevoke }: HostRowProps) {
  const state = useHostDotState(host.id)

  return (
    <div
      data-testid="host-row"
      className="flex items-center gap-3 rounded-lg px-1 py-2 transition-colors hover:bg-muted/50"
    >
      <HostStatusIcon hostId={host.id} state={state} className="text-muted-foreground" />
      <div className="min-w-0 flex-1">
        <div className="ui-font ui-text-sm truncate text-foreground">{host.name}</div>
        {/* Local is the app itself: its line is never tinted. */}
        <div {...(isLocal ? { className: SECONDARY } : hostToneText(state, SECONDARY))}>
          {isLocal ? 'This computer' : host.endpoint}
          {state !== 'up' && ` · ${hostDotLabel(state)}`}
        </div>
      </div>
      {!isLocal && (
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          disabled={revoking}
          aria-label={`Revoke ${host.name}`}
          tooltip={`Revoke ${host.name}`}
          className="text-muted-foreground hover:text-destructive"
          onClick={() => onRevoke(host)}
        >
          <Trash />
        </Button>
      )}
    </div>
  )
}

/** Settings → Hosts: Local and every paired daemon with its status, Add host, and revoke.
 *  Desktop-only: SETTINGS_TAB_ITEMS leaves it out of a non-Tauri build. */
export function HostsSettings() {
  const hosts = useHostConfigStore((s) => s.hosts)
  const [addOpen, setAddOpen] = useState(false)
  // Each opening is a new attempt, even one made while the last is still closing.
  const [addAttempt, setAddAttempt] = useState(0)
  // Every host whose revoke is out: none of them can be asked to revoke twice.
  const [revoking, setRevoking] = useState<ReadonlySet<string>>(() => new Set())

  const handleRevoke = useCallback((host: Host) => {
    setRevoking((ids) => new Set(ids).add(host.id))
    void revokeHost(host).finally(() => {
      setRevoking((ids) => {
        const next = new Set(ids)
        next.delete(host.id)
        return next
      })
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
            revoking={revoking.has(host.id)}
            onRevoke={handleRevoke}
          />
        ))}
      </div>

      <div className="px-1">
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={() => {
            setAddAttempt((n) => n + 1)
            setAddOpen(true)
          }}
        >
          <Plus />
          Add host
        </Button>
      </div>

      <AddHostDialog key={addAttempt} open={addOpen} onOpenChange={setAddOpen} />
    </div>
  )
}
