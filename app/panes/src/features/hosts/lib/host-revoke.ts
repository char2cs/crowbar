import { primitiveConfirm } from '@/components/ui/primitive-dialog-service'
import { hostFailureKind } from '@/features/hosts/lib/classify-host-failure'
import { actionErrorMessage, carriesOnlyStatus } from '@/features/hosts/lib/host-failure-notice'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import type { Host } from '@/features/hosts/types'
import { forgetHostToken, unregisterRemoteHost } from '@/lib/crowbar-bridge'
import { apiFetch } from '@/lib/api/fetch'
import { ApiError } from '@/lib/errors/api-error'
import { userWords } from '@/lib/errors/report'
import { toast } from '@/features/window/stores/toast-store'

export async function revokeHost(host: Host): Promise<boolean> {
  const confirmed = await primitiveConfirm(
    'To use it again you’ll need a new code from Settings → Devices on it (or crowbar pair on a headless host).',
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

function refusedByHost(err: unknown): boolean {
  return hostFailureKind(err) === 'refused' && err instanceof ApiError && !carriesOnlyStatus(err)
}

const FORGETS = 'This computer forgets its key now, so nothing here can use it.'

interface Unrevoked {
  title: string
  why: string
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
      userWords(first, String(first)),
    )
  }
  useHostConfigStore.getState().removeHost(host.id)
}
