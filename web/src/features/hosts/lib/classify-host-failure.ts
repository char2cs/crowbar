import { ApiError, isHostIdentityChangedError } from '@/lib/api'
import type { HostStatus } from '@/features/hosts/types'

/**
 * What a failed request to a remote host says about that host. A changed identity
 * and a refused token (401: revoked, or the host forgot this client) need the user to
 * pair again — no retry fixes them; anything else may pass on its own.
 */
export function classifyHostFailure(err: unknown): Extract<HostStatus, 'down' | 'needs-repair'> {
  if (isHostIdentityChangedError(err)) return 'needs-repair'
  return err instanceof ApiError && err.status === 401 ? 'needs-repair' : 'down'
}
