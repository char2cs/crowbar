import { classifyHostFailure } from '@/features/hosts/lib/classify-host-failure'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { getOrCreateHostRuntime } from '@/features/hosts/stores/host-runtime-registry'
import { LOCAL_HOST } from '@/features/hosts/types'
import { isHostIdentityChangedError, isHostUnreachableError, type ApiError } from '@/lib/api'
import { useConnectionStore } from '@/lib/ws/connection-store'

/**
 * What a failure the desktop proxy marked says about its host, recorded now rather than at
 * the next probe. An unmarked one (a 401 on upgrade, a daemon error) is the prober's to judge.
 */
export function reportHostFailure(err: unknown): void {
  if (!isHostUnreachableError(err) && !isHostIdentityChangedError(err)) return
  const { hostId } = err as ApiError
  if (!hostId || hostId === LOCAL_HOST.id) return
  // A host removed meanwhile gets no runtime back.
  if (!useHostConfigStore.getState().hosts.some((host) => host.id === hostId)) return
  getOrCreateHostRuntime(hostId).getState().setStatus(classifyHostFailure(err))
}

/** Feeds each stream failure the socket manager records into its host's status, once. */
export function followStreamFailures(): () => void {
  const report = (failures: Record<string, ApiError>, counted: Record<string, ApiError>) => {
    for (const [endpoint, failure] of Object.entries(failures)) {
      if (counted[endpoint] !== failure) reportHostFailure(failure)
    }
  }
  report(useConnectionStore.getState().failures, {})
  return useConnectionStore.subscribe((state, prev) => {
    if (state.failures !== prev.failures) report(state.failures, prev.failures)
  })
}
