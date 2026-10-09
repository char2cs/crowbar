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

/** Calls `onUp(hostId)` each time a host's status becomes 'up' from anything else. */
export function onHostUp(onUp: (hostId: string) => void): () => void {
  const watched = new Map<HostRuntimeStore, () => void>()
  const follow = () => {
    const live = new Set(registry.values())
    for (const [store, stop] of watched) {
      if (live.has(store)) continue
      stop()
      watched.delete(store)
    }
    for (const [hostId, store] of registry) {
      if (watched.has(store)) continue
      const stop = store.subscribe((state, prev) => {
        if (state.status === 'up' && prev.status !== 'up') onUp(hostId)
      })
      watched.set(store, stop)
    }
  }
  follow()
  const stopRegistry = subscribeHostRuntimeRegistry(follow)
  return () => {
    stopRegistry()
    for (const stop of watched.values()) stop()
    watched.clear()
  }
}
