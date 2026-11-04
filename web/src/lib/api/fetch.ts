import { daemonChanges, noteDaemonChange } from '@/lib/transport/daemon-changes'
import { ApiError } from '@/lib/errors/api-error'
import { UNREACHED } from '@/lib/errors/report'
import { UserFacingError } from '@/lib/errors/user-facing-error'
import { reportDaemonRefusal } from '@/lib/ws/connection-store'
import { isTauri } from '@/lib/crowbar-bridge'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { hasLocalDaemon } from '@/features/hosts/lib/local-daemon'
import { LOCAL_HOST } from '@/features/hosts/types'
import { HostNotConfiguredError, PROXY_HEADER, proxyMarkerError } from '@/lib/api/errors'
import { routeToHost } from '@/lib/api/host-route'

const crowbar = (window as unknown as { __CROWBAR__?: { api?: string } }).__CROWBAR__
const API_BASE: string = crowbar?.api ?? import.meta.env.VITE_API_URL ?? ''

/**
 * The API base for a host: the local daemon's API_BASE, or a registered remote
 * host, which only the desktop app can reach (its bridge, crowbar://<hostId>,
 * completes the pinned-TLS dial).
 */
export function apiBaseFor(hostId: string): string {
  if (hostId === LOCAL_HOST.id) return API_BASE
  if (!isTauri()) {
    throw new Error(`remote host "${hostId}" is only reachable from the desktop app`)
  }
  const known = useHostConfigStore.getState().hosts.some((h) => h.id === hostId)
  if (!known) throw new HostNotConfiguredError(hostId)
  return `crowbar://${hostId}`
}

/**
 * The absolute URL for a daemon-relative path, qualified or not, for callers that
 * build their own `fetch`. Throws for a host this window cannot reach rather than
 * falling back to the local daemon.
 */
export function apiUrl(path: string): string {
  const target = routeToHost(path)
  return `${apiBaseFor(target.hostId)}${target.path}`
}

/** Tunable transient-retry policy for {@link apiFetch}. A `fetch()` *rejection*
 *  (a network/transport error — never an HTTP status) means the request never
 *  reached the daemon; at app launch that is the sidecar's unix socket not yet
 *  accepting connections. Idempotent reads retry across that startup window so a
 *  cold start lands on the workspace instead of crashing or showing an empty
 *  state. Injectable so tests run without real backoff sleeps. */
export interface RetryConfig {
  /** Total attempts including the first (1 = no retry). */
  attempts: number
  /** Exponential backoff base; delay = baseDelayMs * 2^(attempt-1), capped. */
  baseDelayMs: number
  maxDelayMs: number
  sleep?: (ms: number) => Promise<void>
}

// ~5s of bounded backoff (0,100,200,400,800,1000,1000,1000) comfortably covers a
// daemon sidecar's socket-bind window without masking a genuinely-down daemon.
const DEFAULT_RETRY: RetryConfig = { attempts: 8, baseDelayMs: 100, maxDelayMs: 1000 }

const defaultSleep = (ms: number): Promise<void> => new Promise((r) => setTimeout(r, ms))

/** Only idempotent reads auto-retry on transport failure. Mutations have their
 *  own async-202 + subscribe-before-POST resilience and must never be blindly
 *  replayed (at-least-once would double-apply). */
function isIdempotentRead(init?: RequestInit): boolean {
  const method = init?.method?.toUpperCase()
  return method === undefined || method === 'GET'
}

/** Perform a request with transient-transport retry and
 *  error-status handling, returning the raw `Response`.
 *
 *  This is the layer beneath {@link apiFetch}: every v0 route answers the
 *  `{success,data}` envelope EXCEPT `/review/patch`, which answers text/plain
 *  and reports truncation in a response header. That one caller needs the
 *  Response itself — its body is not JSON and its headers carry meaning — but it
 *  needs the retry and the error-envelope decoding just as much, so both live
 *  here rather than being reimplemented beside it. */
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
      // Transport-level failure — the request never produced an HTTP response
      // (daemon not ready / connection refused). Retry idempotent reads with
      // bounded backoff; surface the error once attempts are exhausted.
      if (attempt >= maxAttempts) throw transportFailure(err, read)
      await sleep(Math.min(retry.maxDelayMs, retry.baseDelayMs * 2 ** (attempt - 1)))
      continue
    }

    // Status FIRST, body second. `fetch` resolves on 4xx/5xx, so reading the
    // envelope before looking at the status is how an error payload gets
    // mistaken for a result. The daemon does answer errors with the same
    // {success,error,data} envelope, so the body is still worth reading here —
    // deliberately, on the error path, purely to recover its `error` message.
    //
    // An HTTP error status is a real daemon response, not a transport failure —
    // it is terminal (a 404 is meaningful; a 500 is a genuine server error) and
    // must never be retried.
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
      // A daemon that is migrating or halted refuses everything; that is its state, not this call's.
      reportDaemonRefusal(refusal.problem)
      throw refusal
    }
    return res
  }
}

/** A write whose answer never came: the daemon may have applied it before the drop. */
const ANSWER_LOST =
  'The connection dropped, so the change may or may not have been applied. Check before trying again.'

// fetch rejects with a TypeError, worded per engine, for a request that got no answer.
// A read was retried; a write may have landed, so it never says to try again.
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

/** The envelope in a body, or null for one that is not JSON (a proxy's HTML
 *  error page, an empty reply): the caller then names the status instead. */
async function parseBody(res: Response): Promise<EnvelopeBody | null> {
  try {
    return (await res.json()) as EnvelopeBody
  } catch {
    return null
  }
}

async function readEnvelope(res: Response): Promise<Envelope> {
  // A 204, or a 202 with no body (a write accepted with no payload), is success with no data.
  const body = res.status === 204 ? null : await parseBody(res)
  return { status: res.status, statusText: res.statusText, body }
}

/** Plain GETs in flight, by `hostId:path`, with the change count they were
 *  sent at: two hosts' reads of the same path must not collide. */
const readsInFlight = new Map<string, { changes: number; envelope: Promise<Envelope> }>()

/** A plain GET, shared with an identical one still in flight when nothing the
 *  client could know of has changed since that one was sent. Joiners get a
 *  copy: callers may hand their answer to a store that freezes it. */
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
