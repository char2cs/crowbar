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

type Saved = Partial<Pick<HostConfigState, 'hosts' | 'remoteEverPaired'>>

/** State saved before `remoteEverPaired` existed lacks it, so a configured host counts too. */
export function hasPairedRemote(s: Pick<HostConfigState, 'hosts' | 'remoteEverPaired'>): boolean {
  return s.remoteEverPaired || s.hosts.length > 0
}

const STORAGE_KEY = 'crowbar.hosts'

/**
 * What is saved, over what this window holds. Every window saves to one key, so the saved
 * host list is the newest word, a removal included (keeping a host the saved list lacks
 * would bring back one revoked elsewhere); `remoteEverPaired` stays one-way.
 */
function withSaved<S extends HostConfigState>(saved: Saved | undefined, current: S): S {
  return {
    ...current,
    hosts: saved?.hosts ?? current.hosts,
    remoteEverPaired: current.remoteEverPaired || saved?.remoteEverPaired === true,
  }
}

/** This window's state brought up to what is saved now: another window may have paired or
 *  removed a host since, and a write from stale state would lose that. */
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
    }),
    {
      name: STORAGE_KEY,
      partialize: (s) => ({ hosts: s.hosts, remoteEverPaired: s.remoteEverPaired }),
      merge: (saved, state) => withSaved(saved as Saved | undefined, state),
    },
  ),
)

/** Calls `onRemoved(id)` for each host that leaves the list, whichever window removed it:
 *  whatever holds a host's data lets it go here, so no removal path can leave it behind. */
export function onHostRemoved(onRemoved: (hostId: string) => void): () => void {
  return useHostConfigStore.subscribe((state, prev) => {
    if (state.hosts === prev.hosts) return
    for (const host of prev.hosts) {
      if (!state.hosts.some((h) => h.id === host.id)) onRemoved(host.id)
    }
  })
}

// The browser tells every other window when one saves: follow it.
window.addEventListener('storage', (event) => {
  if (event.key === STORAGE_KEY) void useHostConfigStore.persist.rehydrate()
})
