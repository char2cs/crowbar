import { apiFetch, isHostIdentityChangedError } from '@/lib/api'
import { getOrCreateHostRuntime } from '@/features/hosts/stores/host-runtime-registry'

/** Probes one host's /v0/health and records the result on its runtime store. */
export async function probeHost(hostId: string): Promise<void> {
  const runtime = getOrCreateHostRuntime(hostId)
  runtime.getState().setStatus('probing')
  const started = performance.now()
  try {
    await apiFetch<void>(
      '/v0/health',
      undefined,
      { attempts: 1, baseDelayMs: 0, maxDelayMs: 0 },
      hostId,
    )
    runtime.getState().setStatus('up', performance.now() - started)
  } catch (err) {
    runtime.getState().setStatus(isHostIdentityChangedError(err) ? 'needs-repair' : 'down')
  }
}

/** Probes hostId immediately and on an interval; returns a stop function. */
export function startHostProbing(hostId: string, intervalMs = 15_000): () => void {
  void probeHost(hostId)
  const timer = setInterval(() => void probeHost(hostId), intervalMs)
  return () => clearInterval(timer)
}
