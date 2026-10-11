import { hostFailureKind } from '@/features/hosts/lib/classify-host-failure'
import { reportHostFailure } from '@/features/hosts/lib/host-failures'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { LOCAL_HOST } from '@/features/hosts/types'
import { toast } from '@/features/window/stores/toast-store'
import { HostNotConfiguredError, type ApiError } from '@/lib/api/errors'
import { HostNotResolvedError } from '@/lib/host-path'
import { hostUnavailability } from '@/lib/host-unavailability'
import { useProjectDataStore } from '@/lib/store/projects'

interface HostFailureNotice {
  message: string
  description?: string
}

const PAIR_AGAIN =
  'To use it again, run `crowbar pair` on that host, then remove it and add it back in Settings → Hosts.'

const NOT_AVAILABLE: HostFailureNotice = {
  message: 'This project’s host is not available',
  description: 'Check the host in Settings → Hosts.',
}

// What to say when a project's host cannot be told: gone, on a host that is not configured,
// or not known until the project list is read.
function unresolvedHostNotice(projectId: string | undefined): HostFailureNotice {
  const why = projectId === undefined ? null : hostUnavailability({ projectId })
  if (why === 'host-unavailable') return NOT_AVAILABLE
  if (why === 'project-missing') return { message: 'This project is no longer in your list' }
  // Before the list is read a local project is merely not known yet: Hosts is the wrong advice.
  if (useProjectDataStore.getState().data.status === 'error') {
    return {
      message: 'Couldn’t load your projects',
      description: 'Check that Crowbar can reach its daemon, then retry.',
    }
  }
  return { message: 'Still loading your projects', description: 'Try again in a moment.' }
}

function describeHostFailure(err: unknown): HostFailureNotice | null {
  if (err instanceof HostNotResolvedError) {
    return unresolvedHostNotice(err.entity === 'project' ? err.id : err.projectId)
  }
  if (err instanceof HostNotConfiguredError) return NOT_AVAILABLE
  const kind = hostFailureKind(err)
  const { hostId } = err as ApiError
  if (kind === null || !hostId || hostId === LOCAL_HOST.id) return null
  // Named by the user's own name for it; a host removed meanwhile has none left to show.
  const name = useHostConfigStore.getState().hosts.find((host) => host.id === hostId)?.name
  if (name === undefined) return NOT_AVAILABLE
  switch (kind) {
    case 'unreachable':
      return {
        message: `Couldn’t reach ${name}`,
        description: 'Check that it is online in Settings → Hosts.',
      }
    // Each message carries what to do on its own: an inline or described surface shows only it.
    case 'identity-changed':
      return {
        message: `${name}’s identity changed — it must be paired again`,
        description: `It no longer presents the certificate you paired with, so Crowbar refused to connect — it may not be your host. ${PAIR_AGAIN}`,
      }
    case 'refused':
      return {
        message: `${name} needs to be paired again`,
        description: `It no longer accepts this computer’s credential. ${PAIR_AGAIN}`,
      }
  }
}

// Every action's failure passes here, so the host's status learns of it as the user does.
function hostFailureNotice(err: unknown): HostFailureNotice | null {
  reportHostFailure(err)
  return describeHostFailure(err)
}

function show({ message, description }: HostFailureNotice): void {
  if (description === undefined) toast.error(message)
  else toast.error(message, description)
}

/** Toasts why nothing can be asked of `projectId`'s host yet, or any more. */
export function toastUnresolvedHost(projectId: string): void {
  show(unresolvedHostNotice(projectId))
}

/** Toasts `err` as its host's failure when the host is why, and says whether it did; anything
 *  else is the caller's to show as it always has. */
export function toastHostFailure(err: unknown): boolean {
  const notice = hostFailureNotice(err)
  if (notice) show(notice)
  return notice !== null
}

/** Toasts a failed action: its host's notice when the host is why, else the error's own
 *  words (`fallback` for a non-Error). */
export function toastActionFailure(err: unknown, fallback: string): void {
  if (!toastHostFailure(err)) toast.error(err instanceof Error ? err.message : fallback)
}

/** The words for a failed action shown inline or under the action's own title: its host's
 *  trouble when the host is why, else the error's own words (`fallback` for a non-Error). */
export function actionErrorMessage<F extends string | undefined>(
  err: unknown,
  fallback: F,
): string | F {
  return hostFailureNotice(err)?.message ?? (err instanceof Error ? err.message : fallback)
}
