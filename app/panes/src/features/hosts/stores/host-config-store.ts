import { create } from 'zustand'
import { persist } from 'zustand/middleware'
import type { Host } from '@/features/hosts/types'

interface HostConfigState {
  hosts: Host[]
  remoteEverPaired: boolean
  addHost: (host: Host) => void
  removeHost: (id: string) => void
  adoptHosts: (hosts: Host[]) => void
}

type Saved = Partial<Pick<HostConfigState, 'hosts' | 'remoteEverPaired'>>

export function hasPairedRemote(s: Pick<HostConfigState, 'hosts' | 'remoteEverPaired'>): boolean {
  return s.remoteEverPaired || s.hosts.length > 0
}

const STORAGE_KEY = 'crowbar.hosts'

function withSaved<S extends HostConfigState>(saved: Saved | undefined, current: S): S {
  return {
    ...current,
    hosts: saved?.hosts ?? current.hosts,
    remoteEverPaired: current.remoteEverPaired || saved?.remoteEverPaired === true,
  }
}

function current(state: HostConfigState): HostConfigState {
  const saved = useHostConfigStore.persist.getOptions().storage?.getItem(STORAGE_KEY)
  return saved && !(saved instanceof Promise) ? withSaved(saved.state as Saved, state) : state
}

export const useHostConfigStore = create<HostConfigState>()(
  persist(
    (set) => ({
      hosts: [],
      remoteEverPaired: false,
      addHost: (host) =>
        set((s) => {
          if (host.id === 'local') return s
          const { hosts } = current(s)
          return { hosts: [...hosts.filter((h) => h.id !== host.id), host], remoteEverPaired: true }
        }),
      removeHost: (id) =>
        set((s) => {
          const { hosts, remoteEverPaired } = current(s)
          return { hosts: hosts.filter((h) => h.id !== id), remoteEverPaired }
        }),
      adoptHosts: (hosts) =>
        set((s) => ({ hosts, remoteEverPaired: s.remoteEverPaired || hosts.length > 0 })),
    }),
    {
      name: STORAGE_KEY,
      partialize: (s) => ({ hosts: s.hosts, remoteEverPaired: s.remoteEverPaired }),
      merge: (saved, state) => withSaved(saved as Saved | undefined, state),
    },
  ),
)

export function onHostRemoved(onRemoved: (hostId: string) => void): () => void {
  return useHostConfigStore.subscribe((state, prev) => {
    if (state.hosts === prev.hosts) return
    for (const host of prev.hosts) {
      if (!state.hosts.some((h) => h.id === host.id)) onRemoved(host.id)
    }
  })
}

window.addEventListener('storage', (event) => {
  if (event.key === STORAGE_KEY) void useHostConfigStore.persist.rehydrate()
})
