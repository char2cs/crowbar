import { daemonChanges, noteDaemonChange } from '@/lib/transport/daemon-changes'
import { ApiError } from '@/lib/errors/api-error'
import { UNREACHED } from '@/lib/errors/report'
import { UserFacingError } from '@/lib/errors/user-facing-error'
import { reportDaemonRefusal } from '@/lib/ws/connection-store'

const crowbar = (window as unknown as { __CROWBAR__?: { api?: string } }).__CROWBAR__
export const API_BASE: string = crowbar?.api ?? import.meta.env.VITE_API_URL ?? ''

/**
 * The desktop proxy's own answer when nothing is listening on the daemon
 * socket (api_proxy.rs: a 503 carrying this header, text/plain, no envelope)
 * — the daemon is starting or respawning, not refusing. Named so UI copy can
 * say so; deliberately NOT retried in apiFetchRaw, the proxy already spent
 * the idempotent-read connect budget.
 */
const PROXY_HEADER = 'x-crowbar-proxy'
const DAEMON_UNAVAILABLE_CODE = 'daemon_unavailable'

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
): Promise<Response> {
  const read = isIdempotentRead(init)
  if (!read) noteDaemonChange()
  const maxAttempts = read ? Math.max(1, retry.attempts) : 1
  const sleep = retry.sleep ?? defaultSleep

  for (let attempt = 1; ; attempt++) {
    let res: Response
    try {
      res = await fetch(`${API_BASE}${path}`, init)
    } catch (err) {
      // Transport-level failure — the request never produced an HTTP response
      // (daemon not ready / connection refused). Retry idempotent reads with
      // bounded backoff; surface the error once attempts are exhausted.
      if (attempt >= maxAttempts) throw unreached(err)
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
      if (res.headers?.get(PROXY_HEADER) === 'daemon-unavailable') {
        throw new ApiError('the daemon is not running yet', res.status, DAEMON_UNAVAILABLE_CODE)
      }
      const errorBody = await parseBody(res)
      if (errorBody === null) {
        throw new ApiError(`${res.status} ${res.statusText}`, res.status, 'internal')
      }
      const refusal = new ApiError(
        errorBody.error ?? `${res.status} ${res.statusText}`,
        res.status,
        typeof errorBody.code === 'string' ? errorBody.code : undefined,
        errorBody.data,
      )
      // A daemon that is migrating or halted refuses everything; that is its state, not this call's.
      reportDaemonRefusal(refusal.problem)
      throw refusal
    }
    return res
  }
}

// fetch rejects a request that reached no server with a TypeError, worded per engine.
function unreached(err: unknown): unknown {
  return err instanceof TypeError ? new UserFacingError(UNREACHED, { cause: err }) : err
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

/** Plain GETs in flight, by path, with the change count they were sent at. */
const readsInFlight = new Map<string, { changes: number; envelope: Promise<Envelope> }>()

/** A plain GET, shared with an identical one still in flight when nothing the
 *  client could know of has changed since that one was sent. Joiners get a
 *  copy: callers may hand their answer to a store that freezes it. */
async function sharedRead(path: string, retry: RetryConfig): Promise<Envelope> {
  const held = readsInFlight.get(path)
  if (held && held.changes === daemonChanges()) return structuredClone(await held.envelope)
  const envelope = apiFetchRaw(path, undefined, retry).then(readEnvelope)
  const entry = { changes: daemonChanges(), envelope }
  readsInFlight.set(path, entry)
  const release = () => {
    if (readsInFlight.get(path) === entry) readsInFlight.delete(path)
  }
  envelope.then(release, release)
  return envelope
}

export async function apiFetch<T>(
  path: string,
  init?: RequestInit,
  retry: RetryConfig = DEFAULT_RETRY,
): Promise<T> {
  const { status, statusText, body } =
    init === undefined
      ? await sharedRead(path, retry)
      : await readEnvelope(await apiFetchRaw(path, init, retry))
  if (body === null) {
    return undefined as T
  }
  if (!body.success) {
    throw new ApiError(
      body.error ?? `${status} ${statusText}`,
      status,
      typeof body.code === 'string' ? body.code : undefined,
      body.data,
    )
  }
  return body.data as T
}
