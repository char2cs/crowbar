import { create } from 'zustand'
import { persist } from 'zustand/middleware'
import type { Host } from '@/features/hosts/types'

interface HostConfigState {
  hosts: Host[]
  /** One-way: set by the first remote `addHost`, never reset (not even by
   *  removing the last host). Remote ids outlive their host, so only "never
   *  paired" proves an unknown id is local — read it through `hasPairedRemote`. */
  remoteEverPaired: boolean
  addHost: (host: Host) => void
  removeHost: (id: string) => void
}

/** State saved before `remoteEverPaired` existed lacks it, so a configured host counts too. */
export function hasPairedRemote(s: Pick<HostConfigState, 'hosts' | 'remoteEverPaired'>): boolean {
  return s.remoteEverPaired || s.hosts.length > 0
}

export const useHostConfigStore = create<HostConfigState>()(
  persist(
    (set) => ({
      hosts: [],
      remoteEverPaired: false,
      addHost: (host) =>
        set((s) =>
          host.id === 'local'
            ? s
            : { hosts: [...s.hosts.filter((h) => h.id !== host.id), host], remoteEverPaired: true },
        ),
      removeHost: (id) => set((s) => ({ hosts: s.hosts.filter((h) => h.id !== id) })),
    }),
    {
      name: 'crowbar.hosts',
      partialize: (s) => ({ hosts: s.hosts, remoteEverPaired: s.remoteEverPaired }),
    },
  ),
)
