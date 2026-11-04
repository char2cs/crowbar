import { create, type StoreApi, type UseBoundStore } from 'zustand'
import type { HostStatus } from '@/features/hosts/types'

interface HostRuntimeState {
  status: HostStatus
  setStatus: (status: HostStatus) => void
}

export type HostRuntimeStore = UseBoundStore<StoreApi<HostRuntimeState>>

function createHostRuntimeStore(): HostRuntimeStore {
  return create<HostRuntimeState>((set) => ({
    status: 'unknown',
    setStatus: (status) => set({ status }),
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
