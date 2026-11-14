import { ApiError } from '@/lib/errors/api-error'
import type { ProblemCode } from '@/lib/errors/problem'

/** A request named a remote host this window holds no pairing for: removed, or never paired. */
export class HostNotConfiguredError extends Error {
  constructor(readonly hostId: string) {
    super(`unknown host "${hostId}"`)
    this.name = 'HostNotConfiguredError'
  }
}

// The desktop proxy's own answers (api_proxy.rs), named by this header. None is
// retried: the proxy already spent its connect budget, and an identity change is
// a hard reject that must never read as plain unreachable (spec §3).
export const PROXY_HEADER = 'x-crowbar-proxy'
const DAEMON_UNAVAILABLE_CODE = 'daemon_unavailable' satisfies ProblemCode
const HOST_UNREACHABLE_CODE = 'host_unreachable' satisfies ProblemCode
const HOST_IDENTITY_CHANGED_CODE = 'host_identity_changed' satisfies ProblemCode

export function isHostUnreachableError(err: unknown): boolean {
  return err instanceof ApiError && err.problem.code === HOST_UNREACHABLE_CODE
}

export function isHostIdentityChangedError(err: unknown): boolean {
  return err instanceof ApiError && err.problem.code === HOST_IDENTITY_CHANGED_CODE
}

// `status` is what the proxy answers with for that marker; an HTTP response
// carries its own, a rejected WebSocket upgrade has none and takes this one.
const PROXY_FAILURES = new Map([
  [
    'daemon-unavailable',
    { code: DAEMON_UNAVAILABLE_CODE, message: 'the daemon is not running yet', status: 503 },
  ],
  [
    'host-unreachable',
    { code: HOST_UNREACHABLE_CODE, message: 'the remote host is unreachable', status: 503 },
  ],
  [
    'host-identity-changed',
    {
      code: HOST_IDENTITY_CHANGED_CODE,
      message: "this host's identity changed — its certificate no longer matches what was pinned",
      status: 502,
    },
  ],
])

/** The error a proxy marker names, or null for a response the proxy did not mark. */
export function proxyMarkerError(
  marker: string | null | undefined,
  hostId: string,
  status?: number,
): ApiError | null {
  const failure = marker ? PROXY_FAILURES.get(marker) : undefined
  if (!failure) return null
  return new ApiError(failure.message, status ?? failure.status, failure.code, undefined, hostId)
}

// A failed remote upgrade has no response headers, so ws_open rejects with
// `<marker>: <reason>` (ws_bridge/remote.rs) — the same markers as above.
const STREAM_MARKERS = ['host-unreachable', 'host-identity-changed']

/** The host failure a rejected stream open names, or null when it names none. */
export function streamFailureOf(err: unknown, hostId: string): ApiError | null {
  const text = err instanceof Error ? err.message : err
  if (typeof text !== 'string') return null
  const marker = STREAM_MARKERS.find((m) => text.startsWith(`${m}: `))
  return proxyMarkerError(marker, hostId)
}
