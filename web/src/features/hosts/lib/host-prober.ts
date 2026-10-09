import { hostsRegistered } from '@/features/hosts/lib/host-registration'
import { probeHost } from '@/features/hosts/lib/host-probe'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { getOrCreateHostRuntime } from '@/features/hosts/stores/host-runtime-registry'

// A host that went down or came back shows within one interval, and N paired hosts cost
// N small requests per interval.
const HOST_PROBE_INTERVAL_MS = 15_000

/**
 * Probes every configured host for as long as it stays configured: the app's one source
 * of each host's status. One probe at a time per host, the next an interval after the
 * last answered, so a host that hangs costs one request, never a pile.
 */
export function startHostProber(): () => void {
  const loops = new Map<string, () => void>()
  const follow = () => {
    const configured = new Set(useHostConfigStore.getState().hosts.map((host) => host.id))
    for (const [hostId, stop] of loops) {
      if (configured.has(hostId)) continue
      stop()
      loops.delete(hostId)
    }
    for (const hostId of configured) {
      if (!loops.has(hostId)) loops.set(hostId, probeLoop(hostId))
    }
  }
  follow()
  const stopFollowing = useHostConfigStore.subscribe(follow)
  return () => {
    stopFollowing()
    for (const stop of loops.values()) stop()
    loops.clear()
  }
}

function probeLoop(hostId: string): () => void {
  let stopped = false
  let timer: ReturnType<typeof setTimeout> | undefined
  const tick = async () => {
    const runtime = getOrCreateHostRuntime(hostId).getState()
    // Only a host never heard from is "checking": a known status stands until the answer.
    if (runtime.status === 'unknown') runtime.setStatus('probing')
    const probe = await probeHost(hostId)
    // A host removed meanwhile is not written to: its runtime stays gone.
    if (stopped) return
    getOrCreateHostRuntime(hostId).getState().setStatus(probe.status, probe.latencyMs)
    timer = setTimeout(() => void tick(), HOST_PROBE_INTERVAL_MS)
  }
  void hostsRegistered().then(() => {
    if (!stopped) void tick()
  })
  return () => {
    stopped = true
    clearTimeout(timer)
  }
}
