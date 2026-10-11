import { classifyHostFailure, hostFailureKind } from '@/features/hosts/lib/classify-host-failure'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { getOrCreateHostRuntime } from '@/features/hosts/stores/host-runtime-registry'
import { LOCAL_HOST } from '@/features/hosts/types'
import type { ApiError } from '@/lib/api/errors'
import { useConnectionStore } from '@/lib/ws/connection-store'

/**
 * What a failure says about its host — not reached, another identity, or this client refused
 * (HTTP 401) — recorded now rather than at the next probe. Anything else (a daemon's own
 * error, an upgrade refused without a status) is the prober's to judge.
 */
export function reportHostFailure(err: unknown): void {
  if (hostFailureKind(err) === null) return
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
