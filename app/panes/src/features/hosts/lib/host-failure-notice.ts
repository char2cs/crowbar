import { hostFailureKind } from '@/features/hosts/lib/classify-host-failure'
import { reportHostFailure } from '@/features/hosts/lib/host-failures'
import { getHostRuntime } from '@/features/hosts/stores/host-runtime-registry'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { LOCAL_HOST } from '@/features/hosts/types'
import { toast } from '@/features/window/stores/toast-store'
import { HostNotConfiguredError, streamFailureOf } from '@/lib/api/errors'
import { ApiError } from '@/lib/errors/api-error'
import { HostNotResolvedError, type HostUnavailable } from '@/lib/host-path'

interface HostFailureNotice {
  message: string
  description?: string
}

const PAIR_AGAIN =
  'To use it again, run crowbar pair on that host, then remove it and add it back in Settings → Hosts.'

const PROJECT_HOST_UNAVAILABLE: HostFailureNotice = {
  message: 'This project’s host is not available',
  description: 'Check the host in Settings → Hosts.',
}

// A request for a host removed meanwhile, whatever it was for: there is no name left to show.
const HOST_NOT_PAIRED: HostFailureNotice = {
  message: 'That host is no longer paired with this computer',
  description: 'Pair it again in Settings → Hosts to use it.',
}

// What to say when a project's host cannot be told: gone, on a host that is not configured,
// or not known until the project list is read.
function unresolvedHostNotice(unavailable: HostUnavailable | null): HostFailureNotice {
  if (unavailable === 'host-unavailable') return PROJECT_HOST_UNAVAILABLE
  if (unavailable === 'project-missing')
    return { message: 'This project is no longer in your list' }
  // Before the list is read a local project is merely not known yet: Hosts is the wrong advice.
  return { message: 'Still loading your projects', description: 'Try again in a moment.' }
}

/** The remote host `err` is a failure of, if any. */
function failedRemoteHost(err: unknown): string | undefined {
  if (!(err instanceof ApiError) || !err.hostId || err.hostId === LOCAL_HOST.id) return undefined
  return err.hostId
}

type NoticeKind = NonNullable<ReturnType<typeof hostFailureKind>> | 'unexplained'

// `hostName` is the failing remote host's, undefined once it is no longer paired.
function describeHostFailure(err: unknown, hostName: string | undefined): HostFailureNotice | null {
  if (err instanceof HostNotResolvedError) return unresolvedHostNotice(err.unavailable)
  if (err instanceof HostNotConfiguredError) return HOST_NOT_PAIRED
  if (!(err instanceof ApiError) || failedRemoteHost(err) === undefined) return null
  const kind = hostFailureKind(err) ?? (carriesOnlyStatus(err) ? 'unexplained' : null)
  if (kind === null) return null
  return hostName === undefined ? HOST_NOT_PAIRED : kindNotice(kind, hostName, err.status)
}

function kindNotice(kind: NoticeKind, name: string, status: number): HostFailureNotice {
  switch (kind) {
    case 'unreachable':
      return {
        message: `Couldn’t reach ${name}`,
        description: 'Check that it is online in Settings → Hosts.',
      }
    // Each message carries what to do on its own: an inline or described surface shows only it.
    case 'identity-changed':
      return {
        message: `${name} is presenting a different identity — confirm it before pairing again`,
        description: `It may have been reinstalled or reset, or someone may be intercepting the connection, so Crowbar refused to connect. Only pair it again after you confirm its new fingerprint with whoever runs it: run crowbar pair on that host, then remove it and add it back in Settings → Hosts.`,
      }
    case 'refused':
      return {
        message: `${name} needs to be paired again`,
        description: `It doesn’t accept this computer’s credential, or none is stored for it here. ${PAIR_AGAIN}`,
      }
    case 'unexplained':
      return {
        message: `${name} returned an error (${status})`,
        description: 'It, or the connection to it, failed this request. Try again.',
      }
  }
}

function hostNameOf(hostId: string | undefined): string | undefined {
  return useHostConfigStore.getState().hosts.find((host) => host.id === hostId)?.name
}

/** Whether `err` is the proxy's own plain-text answer (a 401/500/502/504): no daemon words,
 *  only the status. */
export function carriesOnlyStatus(err: ApiError): boolean {
  return err.message === String(err.status) || err.message.startsWith(`${err.status} `)
}

// Every action's failure passes here, so the host's status learns of it as the user does.
function hostFailureNotice(err: unknown): HostFailureNotice | null {
  reportHostFailure(err)
  return describeHostFailure(err, hostNameOf(failedRemoteHost(err)))
}

function show({ message, description }: HostFailureNotice): void {
  if (description === undefined) toast.error(message)
  else toast.error(message, description)
}

/** Toasts why nothing can be asked of a project's host yet (`unavailable` null), or any more. */
export function toastUnresolvedHost(unavailable: HostUnavailable | null): void {
  show(unresolvedHostNotice(unavailable))
}

/** Its host's words for a failed action when the host is why, else null. */
export function hostFailureWords(err: unknown): string | null {
  return hostFailureNotice(err)?.message ?? null
}

/** Its host's words for a stream to `hostId` that would not open, when the host is why: what
 *  the open names (not reached, another identity), else a refusal its status records. A 401
 *  upgrade names no reason of its own. */
export function streamFailureWords(hostId: string, err: unknown): string | undefined {
  const named = streamFailureOf(err instanceof Error ? err.cause : err, hostId)
  if (named) return hostFailureWords(named) ?? undefined
  if (getHostRuntime(hostId)?.getState().status !== 'needs-repair') return undefined
  const name = hostNameOf(hostId)
  return name === undefined ? HOST_NOT_PAIRED.message : kindNotice('refused', name, 401).message
}

/** Toasts `err` as its host's failure when the host is why, and says whether it did; anything
 *  else is the caller's to show as it always has. */
export function toastHostFailure(err: unknown): boolean {
  const notice = hostFailureNotice(err)
  if (notice) show(notice)
  return notice !== null
}

/** The words for a failed action shown inline or under the action's own title: its host's
 *  trouble when the host is why, else the error's own words (`fallback` for a non-Error). */
export function actionErrorMessage<F extends string | undefined>(
  err: unknown,
  fallback: F,
): string | F {
  return hostFailureWords(err) ?? (err instanceof Error ? err.message : fallback)
}
