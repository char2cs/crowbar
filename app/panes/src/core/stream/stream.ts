import { onHostRemoved, useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { LOCAL_HOST } from '@/features/hosts/types'
import { hasLocalDaemon } from '@/features/hosts/lib/local-daemon'
import { HostStream, type ScopeKindHandler, type ScopeWatch } from '@/core/stream/host-stream'

const handlers = new Map<string, ScopeKindHandler>()
const streams = new Map<string, HostStream>()

function streamOf(hostId: string): HostStream {
  let s = streams.get(hostId)
  if (!s) {
    s = new HostStream(hostId, handlers)
    streams.set(hostId, s)
  }
  return s
}

export function registerScopeKind(kind: string, h: ScopeKindHandler): () => void {
  handlers.set(kind, h)
  for (const s of streams.values()) if (s.epoch !== undefined) h.epoch?.(s.hostId, s.epoch)
  return () => {
    if (handlers.get(kind) === h) handlers.delete(kind)
  }
}

export function holdScope(hostId: string, scope: string): () => void {
  return streamOf(hostId).hold(scope)
}

export function resubscribe(hostId: string, scope: string): void {
  streams.get(hostId)?.resubscribe(scope)
}

export function holding(hostId: string, scope: string): boolean {
  return streams.get(hostId)?.holding(scope) ?? false
}

export function watchScope(hostId: string, scope: string, watch: ScopeWatch): () => void {
  return streamOf(hostId).watch(scope, watch)
}

export function onSignal<T>(
  hostId: string,
  scope: string,
  kind: string,
  cb: (data: T) => void,
): () => void {
  return streamOf(hostId).onSignal(scope, kind, (data) => cb(data as T))
}

export function streamOpened(hostId: string): Promise<void> {
  return streamOf(hostId).opened()
}

function stopHost(hostId: string): void {
  const s = streams.get(hostId)
  if (!s) return
  s.stop()
  streams.delete(hostId)
  for (const h of handlers.values()) h.forget?.(hostId)
}

function startHostStreams(): () => void {
  const follow = () => {
    for (const host of useHostConfigStore.getState().hosts) streamOf(host.id).start()
  }
  follow()
  const stopAdds = useHostConfigStore.subscribe(follow)
  const stopRemovals = onHostRemoved(stopHost)
  return () => {
    stopAdds()
    stopRemovals()
    for (const host of useHostConfigStore.getState().hosts) streams.get(host.id)?.stop()
  }
}

let running: (() => void) | null = null

export function startStream(): () => void {
  if (running) return running
  const local = hasLocalDaemon() ? streamOf(LOCAL_HOST.id) : null
  local?.start()
  const stopHosts = startHostStreams()
  const stop = () => {
    if (running !== stop) return
    running = null
    stopHosts()
    local?.stop()
  }
  running = stop
  return stop
}
