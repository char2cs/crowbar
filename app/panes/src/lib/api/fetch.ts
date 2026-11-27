import { daemonChanges, noteDaemonChange } from '@/lib/transport/daemon-changes'
import { ApiError } from '@/lib/errors/api-error'
import { UNREACHED } from '@/lib/errors/report'
import { UserFacingError } from '@/lib/errors/user-facing-error'
import { reportDaemonRefusal } from '@/lib/ws/connection-store'
import { isTauri } from '@/lib/crowbar-bridge'
import { inShell } from '@/lib/ws/in-shell'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { hasLocalDaemon } from '@/features/hosts/lib/local-daemon'
import { LOCAL_HOST } from '@/features/hosts/types'
import { HostNotConfiguredError, PROXY_HEADER, proxyMarkerError } from '@/lib/api/errors'
import { routeToHost } from '@/lib/api/host-route'

function localApiBase(): string {
  const crowbar = (window as unknown as { __CROWBAR__?: { api?: string } }).__CROWBAR__
  return crowbar?.api ?? import.meta.env.VITE_API_URL ?? ''
}

export function apiBaseFor(hostId: string): string {
  if (hostId === LOCAL_HOST.id) return localApiBase()
  if (!isTauri() && !inShell()) {
    throw new Error(`remote host "${hostId}" is only reachable from the desktop app`)
  }
  const known = useHostConfigStore.getState().hosts.some((h) => h.id === hostId)
  if (!known) throw new HostNotConfiguredError(hostId)
  return `crowbar://${hostId}`
}

export function apiUrl(path: string): string {
  const target = routeToHost(path)
  return `${apiBaseFor(target.hostId)}${target.path}`
}

export interface RetryConfig {
  attempts: number
  baseDelayMs: number
  maxDelayMs: number
  sleep?: (ms: number) => Promise<void>
}

const DEFAULT_RETRY: RetryConfig = { attempts: 8, baseDelayMs: 100, maxDelayMs: 1000 }

const defaultSleep = (ms: number): Promise<void> => new Promise((r) => setTimeout(r, ms))

function isIdempotentRead(init?: RequestInit): boolean {
  const method = init?.method?.toUpperCase()
  return method === undefined || method === 'GET'
}

export async function apiFetchRaw(
  path: string,
  init?: RequestInit,
  retry: RetryConfig = DEFAULT_RETRY,
  hostId?: string,
): Promise<Response> {
  const target = routeToHost(path, hostId)
  if (target.hostId === LOCAL_HOST.id && !hasLocalDaemon()) {
    throw new ApiError(
      'this install has no local daemon',
      503,
      'daemon_unavailable',
      undefined,
      target.hostId,
    )
  }
  const read = isIdempotentRead(init)
  if (!read) noteDaemonChange()
  const maxAttempts = read ? Math.max(1, retry.attempts) : 1
  const sleep = retry.sleep ?? defaultSleep
  const base = apiBaseFor(target.hostId)

  for (let attempt = 1; ; attempt++) {
    let res: Response
    try {
      res = await fetch(`${base}${target.path}`, init)
    } catch (err) {
      if (attempt >= maxAttempts) throw transportFailure(err, read)
      await sleep(Math.min(retry.maxDelayMs, retry.baseDelayMs * 2 ** (attempt - 1)))
      continue
    }

    if (!res.ok) {
      const proxied = proxyMarkerError(res.headers?.get(PROXY_HEADER), target.hostId, res.status)
      if (proxied) throw proxied
      const errorBody = await parseBody(res)
      if (errorBody === null) {
        throw new ApiError(
          `${res.status} ${res.statusText}`,
          res.status,
          'internal',
          undefined,
          target.hostId,
        )
      }
      const refusal = new ApiError(
        errorBody.error ?? `${res.status} ${res.statusText}`,
        res.status,
        typeof errorBody.code === 'string' ? errorBody.code : undefined,
        errorBody.data,
        target.hostId,
      )
      reportDaemonRefusal(refusal.problem)
      throw refusal
    }
    return res
  }
}

const ANSWER_LOST =
  'The connection dropped, so the change may or may not have been applied. Check before trying again.'

function transportFailure(err: unknown, read: boolean): unknown {
  if (!(err instanceof TypeError)) return err
  return new UserFacingError(read ? UNREACHED : ANSWER_LOST, { cause: err })
}

interface EnvelopeBody {
  success?: boolean
  error?: string
  code?: unknown
  data?: unknown
}

interface Envelope {
  status: number
  statusText: string
  body: EnvelopeBody | null
}

async function parseBody(res: Response): Promise<EnvelopeBody | null> {
  try {
    return (await res.json()) as EnvelopeBody
  } catch {
    return null
  }
}

async function readEnvelope(res: Response): Promise<Envelope> {
  const body = res.status === 204 ? null : await parseBody(res)
  return { status: res.status, statusText: res.statusText, body }
}

const readsInFlight = new Map<string, { changes: number; envelope: Promise<Envelope> }>()

async function sharedRead(path: string, retry: RetryConfig, hostId: string): Promise<Envelope> {
  const key = `${hostId}:${path}`
  const held = readsInFlight.get(key)
  if (held && held.changes === daemonChanges()) return structuredClone(await held.envelope)
  const envelope = apiFetchRaw(path, undefined, retry, hostId).then(readEnvelope)
  const entry = { changes: daemonChanges(), envelope }
  readsInFlight.set(key, entry)
  const release = () => {
    if (readsInFlight.get(key) === entry) readsInFlight.delete(key)
  }
  envelope.then(release, release)
  return envelope
}

export async function apiFetch<T>(
  path: string,
  init?: RequestInit,
  retry: RetryConfig = DEFAULT_RETRY,
  hostId?: string,
): Promise<T> {
  const target = routeToHost(path, hostId)
  const { status, statusText, body } =
    init === undefined
      ? await sharedRead(target.path, retry, target.hostId)
      : await readEnvelope(await apiFetchRaw(target.path, init, retry, target.hostId))
  if (body === null) {
    return undefined as T
  }
  if (!body.success) {
    throw new ApiError(
      body.error ?? `${status} ${statusText}`,
      status,
      typeof body.code === 'string' ? body.code : undefined,
      body.data,
      target.hostId,
    )
  }
  return body.data as T
}
