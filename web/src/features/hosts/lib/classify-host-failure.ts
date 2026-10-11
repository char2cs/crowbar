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

/**
 * What a failed request to a remote host says about that host. A changed identity
 * and a refused token (401: revoked, or the host forgot this client) need the user to
 * pair again — no retry fixes them; anything else may pass on its own.
 */
export function classifyHostFailure(err: unknown): Extract<HostStatus, 'down' | 'needs-repair'> {
  const kind = hostFailureKind(err)
  return kind === 'identity-changed' || kind === 'refused' ? 'needs-repair' : 'down'
}
