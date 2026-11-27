import { isHostIdentityChangedError, isHostUnreachableError } from '@/lib/api/errors'
import { ApiError } from '@/lib/errors/api-error'
import type { HostStatus } from '@/features/hosts/types'

type HostFailureKind = 'unreachable' | 'identity-changed' | 'refused'

export function hostFailureKind(err: unknown): HostFailureKind | null {
  if (isHostIdentityChangedError(err)) return 'identity-changed'
  if (isHostUnreachableError(err)) return 'unreachable'
  return err instanceof ApiError && err.status === 401 ? 'refused' : null
}

export function classifyHostFailure(err: unknown): Extract<HostStatus, 'down' | 'needs-repair'> {
  const kind = hostFailureKind(err)
  return kind === 'identity-changed' || kind === 'refused' ? 'needs-repair' : 'down'
}
