import { ApiError, isHostIdentityChangedError, isHostUnreachableError } from '@/lib/api/errors'
import type { HostStatus } from '@/features/hosts/types'

/** A remote host's own trouble with a request: not reached, not the host that was paired, or
 *  refusing this client (HTTP 401: revoked, or its credential is gone). */
type HostFailureKind = 'unreachable' | 'identity-changed' | 'refused'

/** Which of its host's own troubles failed a request, or null when the host is not why. */
export function hostFailureKind(err: unknown): HostFailureKind | null {
  if (isHostIdentityChangedError(err)) return 'identity-changed'
  if (isHostUnreachableError(err)) return 'unreachable'
  return err instanceof ApiError && err.status === 401 ? 'refused' : null
}

/** What a failed request says about its remote host: a changed identity or a refused token
 *  (401) needs pairing again, which no retry fixes; anything else may pass on its own. */
export function classifyHostFailure(err: unknown): Extract<HostStatus, 'down' | 'needs-repair'> {
  const kind = hostFailureKind(err)
  return kind === 'identity-changed' || kind === 'refused' ? 'needs-repair' : 'down'
}
