import { primitiveConfirm } from '@/components/ui/primitive-dialog-service'
import { hostFailureKind } from '@/features/hosts/lib/classify-host-failure'
import { actionErrorMessage, carriesOnlyStatus } from '@/features/hosts/lib/host-failure-notice'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import type { Host } from '@/features/hosts/types'
import { forgetHostToken, unregisterRemoteHost } from '@/lib/crowbar-bridge'
import { apiFetch } from '@/lib/api'
import { ApiError } from '@/lib/api/errors'
import { toast } from '@/features/window/stores/toast-store'

// A host revokes once at a time: every list that offers the swipe shares one flow and one question.
const inFlight = new Map<string, Promise<boolean>>()

export function revokeHost(host: Host): Promise<boolean> {
  const running = inFlight.get(host.id)
  if (running) return running
  const flow = runRevoke(host).finally(() => inFlight.delete(host.id))
  inFlight.set(host.id, flow)
  return flow
}

// The host's own DELETE (over its channel, never Local's) goes first: cleaning up here first
// would drop the only credential that can ever revoke this client there. Whether the host went.
async function runRevoke(host: Host): Promise<boolean> {
  const confirmed = await primitiveConfirm(
    'To use it again you’ll need to run crowbar pair on it.',
    {
      title: `Revoke ${host.name}?`,
      confirmLabel: 'Revoke',
      cancelLabel: 'Keep it',
      destructive: true,
    },
  )
  if (!confirmed) return false
  try {
    await apiFetch(`/v0/clients/${host.clientId}`, { method: 'DELETE' }, undefined, host.id)
  } catch (err) {
    if (!(await mayRemoveUnrevoked(host, err))) return false
  }
  await removeHere(host)
  return true
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
