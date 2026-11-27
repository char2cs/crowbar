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
  'To use it again, remove it here, then add it back in Settings → Hosts with a new code from Settings → Devices on that host (or crowbar pair on a headless host).'

const PROJECT_HOST_UNAVAILABLE: HostFailureNotice = {
  message: 'This project’s host is not available',
  description: 'Check the host in Settings → Hosts.',
}

const HOST_NOT_PAIRED: HostFailureNotice = {
  message: 'That host is no longer paired with this computer',
  description: 'Pair it again in Settings → Hosts to use it.',
}

function unresolvedHostNotice(unavailable: HostUnavailable | null): HostFailureNotice {
  if (unavailable === 'host-unavailable') return PROJECT_HOST_UNAVAILABLE
  if (unavailable === 'project-missing')
    return { message: 'This project is no longer in your list' }
  return { message: 'Still loading your projects', description: 'Try again in a moment.' }
}

function failedRemoteHost(err: unknown): string | undefined {
  if (!(err instanceof ApiError) || !err.hostId || err.hostId === LOCAL_HOST.id) return undefined
  return err.hostId
}

type NoticeKind = NonNullable<ReturnType<typeof hostFailureKind>> | 'unexplained'

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
    case 'identity-changed':
      return {
        message: `${name} is presenting a different identity — confirm it before pairing again`,
        description: `It may have been reinstalled or reset, or someone may be intercepting the connection, so Crowbar refused to connect. Only pair it again after you confirm its new fingerprint with whoever runs it: remove it here, then add it back in Settings → Hosts with a new code from Settings → Devices on that host (or crowbar pair on a headless host).`,
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

export function carriesOnlyStatus(err: ApiError): boolean {
  return err.message === String(err.status) || err.message.startsWith(`${err.status} `)
}

function hostFailureNotice(err: unknown): HostFailureNotice | null {
  reportHostFailure(err)
  return describeHostFailure(err, hostNameOf(failedRemoteHost(err)))
}

function show({ message, description }: HostFailureNotice): void {
  if (description === undefined) toast.error(message)
  else toast.error(message, description)
}

export function toastUnresolvedHost(unavailable: HostUnavailable | null): void {
  show(unresolvedHostNotice(unavailable))
}

export function hostFailureRefusal(err: unknown): { title: string; message: string } | null {
  const notice = hostFailureNotice(err)
  return notice && { title: notice.message, message: notice.description ?? '' }
}

export function hostFailureWords(err: unknown): string | null {
  return hostFailureNotice(err)?.message ?? null
}

export function streamFailureWords(hostId: string, err: unknown): string | undefined {
  const named = streamFailureOf(err instanceof Error ? err.cause : err, hostId)
  if (named) return hostFailureWords(named) ?? undefined
  if (getHostRuntime(hostId)?.getState().status !== 'needs-repair') return undefined
  const name = hostNameOf(hostId)
  return name === undefined ? HOST_NOT_PAIRED.message : kindNotice('refused', name, 401).message
}

export function toastHostFailure(err: unknown): boolean {
  const notice = hostFailureNotice(err)
  if (notice) show(notice)
  return notice !== null
}

export function actionErrorMessage<F extends string | undefined>(
  err: unknown,
  fallback: F,
): string | F {
  return hostFailureWords(err) ?? (err instanceof Error ? err.message : fallback)
}
