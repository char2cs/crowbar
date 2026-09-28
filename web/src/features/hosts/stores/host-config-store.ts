import { create } from 'zustand'
import { persist } from 'zustand/middleware'
import type { Host } from '@/features/hosts/types'

interface HostConfigState {
  hosts: Host[]
  addHost: (host: Host) => void
  removeHost: (id: string) => void
}

export const useHostConfigStore = create<HostConfigState>()(
  persist(
    (set) => ({
      hosts: [],
      addHost: (host) =>
        set((s) => (host.id === 'local' ? s : { hosts: [...s.hosts.filter((h) => h.id !== host.id), host] })),
      removeHost: (id) => set((s) => ({ hosts: s.hosts.filter((h) => h.id !== id) })),
    }),
    { name: 'crowbar.hosts', partialize: (s) => ({ hosts: s.hosts }) },
  ),
)
