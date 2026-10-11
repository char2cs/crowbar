import { create, type StoreApi, type UseBoundStore } from 'zustand'
import type { HostStatus } from '@/features/hosts/types'

interface HostRuntimeState {
  status: HostStatus
  latencyMs: number | null
  setStatus: (status: HostStatus, latencyMs?: number | null) => void
}

export type HostRuntimeStore = UseBoundStore<StoreApi<HostRuntimeState>>

function createHostRuntimeStore(): HostRuntimeStore {
  return create<HostRuntimeState>((set) => ({
    status: 'unknown',
    latencyMs: null,
    setStatus: (status, latencyMs = null) => set({ status, latencyMs }),
  }))
}

const registry = new Map<string, HostRuntimeStore>()

const listeners = new Set<() => void>()
function notify(): void {
  for (const listener of listeners) listener()
}

function subscribeHostRuntimeRegistry(callback: () => void): () => void {
  listeners.add(callback)
  return () => {
    listeners.delete(callback)
  }
}

export function getOrCreateHostRuntime(hostId: string): HostRuntimeStore {
  if (!registry.has(hostId)) {
    registry.set(hostId, createHostRuntimeStore())
    notify()
  }
  return registry.get(hostId)!
}

export function getHostRuntime(hostId: string): HostRuntimeStore | undefined {
  return registry.get(hostId)
}

export function destroyHostRuntime(hostId: string): void {
  if (registry.delete(hostId)) notify()
}

/** Calls `onChange` when `hostId`'s status changes, following its runtime as it is
 *  minted or destroyed. */
export function subscribeHostStatus(hostId: string, onChange: () => void): () => void {
  let watched = getHostRuntime(hostId)
  let unsubscribe = watched?.subscribe(onChange)
  const stopRegistry = subscribeHostRuntimeRegistry(() => {
    if (getHostRuntime(hostId) === watched) return
    unsubscribe?.()
    watched = getHostRuntime(hostId)
    unsubscribe = watched?.subscribe(onChange)
    onChange()
  })
  return () => {
    stopRegistry()
    unsubscribe?.()
  }
}

/** Calls `onChange` when any host's status changes, or a runtime is minted or destroyed. */
export function subscribeHostStatuses(onChange: () => void): () => void {
  const watched = new Map<HostRuntimeStore, () => void>()
  const follow = () => {
    const live = new Set(registry.values())
    for (const [store, unsubscribe] of watched) {
      if (live.has(store)) continue
      unsubscribe()
      watched.delete(store)
    }
    for (const store of live) if (!watched.has(store)) watched.set(store, store.subscribe(onChange))
  }
  follow()
  const stopRegistry = subscribeHostRuntimeRegistry(() => {
    follow()
    onChange()
  })
  return () => {
    stopRegistry()
    for (const unsubscribe of watched.values()) unsubscribe()
  }
}

interface UpWatch {
  unsubscribe: () => void
  /** What `run` returned for the host's current 'up', while it lasts. */
  stop?: () => void
}

/**
 * Runs `run(hostId)` while a host is 'up': each time it becomes 'up' (at once for one
 * already up), calling what it returned when the host leaves 'up' or its runtime goes.
 */
export function whileHostUp(run: (hostId: string) => () => void): () => void {
  const watched = new Map<HostRuntimeStore, UpWatch>()
  const end = (watch: UpWatch) => {
    const stop = watch.stop
    watch.stop = undefined
    stop?.()
  }
  const follow = () => {
    const live = new Set(registry.values())
    for (const [store, watch] of watched) {
      if (live.has(store)) continue
      watch.unsubscribe()
      watched.delete(store)
      end(watch)
    }
    for (const [hostId, store] of registry) {
      if (watched.has(store)) continue
      const watch: UpWatch = { unsubscribe: () => {} }
      const sync = () => {
        const up = store.getState().status === 'up'
        if (up && !watch.stop) watch.stop = run(hostId)
        else if (!up) end(watch)
      }
      watch.unsubscribe = store.subscribe(sync)
      watched.set(store, watch)
      sync()
    }
  }
  follow()
  const stopRegistry = subscribeHostRuntimeRegistry(follow)
  return () => {
    stopRegistry()
    for (const watch of watched.values()) {
      watch.unsubscribe()
      end(watch)
    }
    watched.clear()
  }
}
