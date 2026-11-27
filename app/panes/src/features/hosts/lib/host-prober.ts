import { hostsRegistered } from '@/features/hosts/lib/host-registration'
import { probeHost } from '@/features/hosts/lib/host-probe'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import {
  destroyHostRuntime,
  getHostRuntime,
  getOrCreateHostRuntime,
  subscribeHostStatus,
} from '@/features/hosts/stores/host-runtime-registry'

const HOST_PROBE_INTERVAL_MS = 15_000

const DOWN_FIRST_RETRY_MS = 1_000

export function startHostProber(): () => void {
  const loops = new Map<string, () => void>()
  const follow = () => {
    const configured = new Set(useHostConfigStore.getState().hosts.map((host) => host.id))
    for (const [hostId, stop] of loops) {
      if (configured.has(hostId)) continue
      stop()
      loops.delete(hostId)
      destroyHostRuntime(hostId)
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
  let registered = false
  let probing = false
  let timer: ReturnType<typeof setTimeout> | undefined
  let downRetry = DOWN_FIRST_RETRY_MS
  const next = (delay: number) => {
    clearTimeout(timer)
    timer = setTimeout(() => void tick(), delay)
  }
  const retryDown = () => {
    next(downRetry)
    downRetry = Math.min(downRetry * 2, HOST_PROBE_INTERVAL_MS)
  }
  const tick = async () => {
    probing = true
    const runtime = getOrCreateHostRuntime(hostId).getState()
    if (runtime.status === 'unknown') runtime.setStatus('probing')
    const probe = await probeHost(hostId)
    if (stopped) return
    getOrCreateHostRuntime(hostId).getState().setStatus(probe.status)
    probing = false
    if (probe.status === 'down') {
      retryDown()
      return
    }
    downRetry = DOWN_FIRST_RETRY_MS
    next(HOST_PROBE_INTERVAL_MS)
  }
  let last = getHostRuntime(hostId)?.getState().status
  const stopWatching = subscribeHostStatus(hostId, () => {
    const status = getHostRuntime(hostId)?.getState().status
    const wentDown = status === 'down' && last !== 'down'
    last = status
    if (wentDown && registered && !probing && !stopped) retryDown()
  })
  void hostsRegistered().then(() => {
    if (stopped) return
    registered = true
    void tick()
  })
  return () => {
    stopped = true
    stopWatching()
    clearTimeout(timer)
  }
}
