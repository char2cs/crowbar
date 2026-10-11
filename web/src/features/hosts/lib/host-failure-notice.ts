import { hostFailureKind } from '@/features/hosts/lib/classify-host-failure'
import { reportHostFailure } from '@/features/hosts/lib/host-failures'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { LOCAL_HOST } from '@/features/hosts/types'
import { toast } from '@/features/window/stores/toast-store'
import { ApiError, HostNotConfiguredError } from '@/lib/api/errors'
import { HostNotResolvedError } from '@/lib/host-path'
import {
  hostUnavailability,
  useHostUnavailable,
  type HostUnavailable,
} from '@/lib/host-unavailability'
import type { Loadable } from '@/lib/loadable'
import { useProjectDataStore } from '@/lib/store/projects'

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

/** What a host failure's words depend on besides the error itself. */
interface HostFailureFacts {
  /** The failing remote host's name; undefined once it is no longer paired. */
  hostName: string | undefined
  /** Why an unresolved project's host is not coming; null while it is worth waiting. */
  unavailable: HostUnavailable | null
  listStatus: Loadable<unknown>['status']
}

// What to say when a project's host cannot be told: gone, on a host that is not configured,
// or not known until the project list is read.
function unresolvedHostNotice({ unavailable, listStatus }: HostFailureFacts): HostFailureNotice {
  if (unavailable === 'host-unavailable') return PROJECT_HOST_UNAVAILABLE
  if (unavailable === 'project-missing')
    return { message: 'This project is no longer in your list' }
  // Before the list is read a local project is merely not known yet: Hosts is the wrong advice.
  if (listStatus === 'error') {
    return {
      message: 'Couldn’t load your projects',
      description: 'Check that Crowbar can reach its daemon, then retry.',
    }
  }
  return { message: 'Still loading your projects', description: 'Try again in a moment.' }
}

/** The remote host `err` is a failure of, if any. */
function failedRemoteHost(err: unknown): string | undefined {
  if (!(err instanceof ApiError) || !err.hostId || err.hostId === LOCAL_HOST.id) return undefined
  return err.hostId
}

/** The project whose host `err` could not tell, if it names one. */
function unresolvedProject(err: unknown): string | undefined {
  if (!(err instanceof HostNotResolvedError)) return undefined
  return err.entity === 'project' ? err.id : err.projectId
}

function describeHostFailure(err: unknown, facts: HostFailureFacts): HostFailureNotice | null {
  if (err instanceof HostNotResolvedError) return unresolvedHostNotice(facts)
  if (err instanceof HostNotConfiguredError) return HOST_NOT_PAIRED
  if (!(err instanceof ApiError) || failedRemoteHost(err) === undefined) return null
  const kind = hostFailureKind(err) ?? (carriesOnlyStatus(err) ? 'unexplained' : null)
  if (kind === null) return null
  const name = facts.hostName
  if (name === undefined) return HOST_NOT_PAIRED
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
        message: `${name} returned an error (${err.status})`,
        description: 'It, or the connection to it, failed this request. Try again.',
      }
  }
}

// For handlers and effects: the stores as they are now.
function currentFacts(hostId: string | undefined, projectId: string | undefined): HostFailureFacts {
  return {
    hostName: useHostConfigStore.getState().hosts.find((host) => host.id === hostId)?.name,
    unavailable: projectId === undefined ? null : hostUnavailability({ projectId }),
    listStatus: useProjectDataStore.getState().data.status,
  }
}

/** Whether `err` is the proxy's own plain-text answer (a 401/500/502/504): no daemon words,
 *  only the status. */
export function carriesOnlyStatus(err: ApiError): boolean {
  return err.message === String(err.status) || err.message.startsWith(`${err.status} `)
}

// Every action's failure passes here, so the host's status learns of it as the user does.
function hostFailureNotice(err: unknown): HostFailureNotice | null {
  reportHostFailure(err)
  return describeHostFailure(err, currentFacts(failedRemoteHost(err), unresolvedProject(err)))
}

function show({ message, description }: HostFailureNotice): void {
  if (description === undefined) toast.error(message)
  else toast.error(message, description)
}

/** Toasts why nothing can be asked of `projectId`'s host yet, or any more. */
export function toastUnresolvedHost(projectId: string): void {
  show(unresolvedHostNotice(currentFacts(undefined, projectId)))
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

/** Its host's words for `err` when the host is why, else null — kept current as the host and
 *  project lists change: for a surface that says what failed in its own words. */
export function useHostFailureMessage(err: unknown): string | null {
  const hostId = failedRemoteHost(err)
  const projectId = unresolvedProject(err)
  const hostName = useHostConfigStore((s) => s.hosts.find((host) => host.id === hostId)?.name)
  const unavailable = useHostUnavailable(projectId === undefined ? null : { projectId })
  const listStatus = useProjectDataStore((s) => s.data.status)
  return describeHostFailure(err, { hostName, unavailable, listStatus })?.message ?? null
}

/** The words for a failed action shown inline or under the action's own title: its host's
 *  trouble when the host is why, else the error's own words (`fallback` for a non-Error). */
export function actionErrorMessage<F extends string | undefined>(
  err: unknown,
  fallback: F,
): string | F {
  return hostFailureNotice(err)?.message ?? (err instanceof Error ? err.message : fallback)
}
