import { onHostRemoved, useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { LOCAL_HOST } from '@/features/hosts/types'
import { hasLocalDaemon } from '@/features/hosts/lib/local-daemon'
import { HostStream, type ScopeKindHandler, type ScopeWatch } from '@/core/stream/host-stream'

/** Every host's stream: one socket per host, each holding scopes by their bare names. */
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

/** A store claims the frames of `kind` on every host; one registered after a host's ready
 *  hears that host's epoch at once. */
export function registerScopeKind(kind: string, h: ScopeKindHandler): () => void {
  handlers.set(kind, h)
  for (const s of streams.values()) if (s.epoch !== undefined) h.epoch?.(s.hostId, s.epoch)
  return () => {
    if (handlers.get(kind) === h) handlers.delete(kind)
  }
}

/** Holds `scope` on `hostId`'s stream until the returned release. */
export function holdScope(hostId: string, scope: string): () => void {
  return streamOf(hostId).hold(scope)
}

/** Asks `hostId` again for a held scope from where its kind says it resumes. */
export function resubscribe(hostId: string, scope: string): void {
  streams.get(hostId)?.resubscribe(scope)
}

/** Whether some holder still holds `scope` on `hostId`: a refused one is let go. */
export function holding(hostId: string, scope: string): boolean {
  return streams.get(hostId)?.holding(scope) ?? false
}

/** Hears `scope`'s synced and refusal frames on `hostId`; one already synced is heard at once. */
export function watchScope(hostId: string, scope: string, watch: ScopeWatch): () => void {
  return streamOf(hostId).watch(scope, watch)
}

/** Hears `scope`'s signals of `kind` on `hostId`; returns the stop. */
export function onSignal<T>(
  hostId: string,
  scope: string,
  kind: string,
  cb: (data: T) => void,
): () => void {
  return streamOf(hostId).onSignal(scope, kind, (data) => cb(data as T))
}

/** Resolves once `hostId`'s socket is open; rejects when the attempt it waits on fails. */
export function streamOpened(hostId: string): Promise<void> {
  return streamOf(hostId).opened()
}

function stopHost(hostId: string): void {
  const s = streams.get(hostId)
  if (!s) return
  // The stream goes first, so no frame of the removed host lands after it is forgotten.
  s.stop()
  streams.delete(hostId)
  for (const h of handlers.values()) h.forget?.(hostId)
}

/** One stream per configured host, following the host list until the returned stop. */
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

/** Opens the local daemon's stream, when this install has one, and every paired host's,
 *  each holding its projects scope for as long as it runs. A second call returns the
 *  first's stop. Called once, at boot. */
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
