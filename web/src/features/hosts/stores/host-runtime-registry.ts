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

export type HostRuntimeRegistryChange = 'registered' | 'destroyed'
const listeners = new Set<(change: HostRuntimeRegistryChange) => void>()
function notify(change: HostRuntimeRegistryChange): void {
  for (const listener of listeners) listener(change)
}

export function subscribeHostRuntimeRegistry(callback: (change: HostRuntimeRegistryChange) => void): () => void {
  listeners.add(callback)
  return () => {
    listeners.delete(callback)
  }
}

export function getOrCreateHostRuntime(hostId: string): HostRuntimeStore {
  if (!registry.has(hostId)) {
    registry.set(hostId, createHostRuntimeStore())
    notify('registered')
  }
  return registry.get(hostId)!
}

export function getHostRuntime(hostId: string): HostRuntimeStore | undefined {
  return registry.get(hostId)
}

export function destroyHostRuntime(hostId: string): void {
  if (registry.delete(hostId)) notify('destroyed')
}

export function getAllRegisteredHostIds(): string[] {
  return Array.from(registry.keys())
}
