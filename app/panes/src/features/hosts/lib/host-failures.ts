import { classifyHostFailure, hostFailureKind } from '@/features/hosts/lib/classify-host-failure'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { getOrCreateHostRuntime } from '@/features/hosts/stores/host-runtime-registry'
import { LOCAL_HOST } from '@/features/hosts/types'
import type { ApiError } from '@/lib/errors/api-error'
import { useConnectionStore } from '@/lib/ws/connection-store'

export function reportHostFailure(err: unknown): void {
  if (hostFailureKind(err) === null) return
  const { hostId } = err as ApiError
  if (!hostId || hostId === LOCAL_HOST.id) return
  if (!useHostConfigStore.getState().hosts.some((host) => host.id === hostId)) return
  getOrCreateHostRuntime(hostId).getState().setStatus(classifyHostFailure(err))
}

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
