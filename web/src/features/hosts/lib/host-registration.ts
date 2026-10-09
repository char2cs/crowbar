import { isTauri, registerRemoteHost } from '@/lib/crowbar-bridge'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'

let registration: Promise<void> | undefined

/**
 * Settles once every saved host is registered with the desktop proxy. Its registry
 * lives in memory and starts empty at each launch, so a remote request sent earlier
 * is refused. Registering records an address and dials nothing: a host that is down
 * delays nothing. The registry is per process, and pairing registers a new host
 * itself, so a host paired in another window needs nothing here. A browser has no
 * proxy: ready at once.
 */
export function hostsRegistered(): Promise<void> {
  registration ??= registerSavedHosts()
  return registration
}

async function registerSavedHosts(): Promise<void> {
  if (!isTauri()) return
  const { hosts } = useHostConfigStore.getState()
  await Promise.all(
    hosts.map((host) =>
      registerRemoteHost(host.id, host.endpoint).catch((err: unknown) => {
        console.error(`the desktop proxy refused host "${host.id}"`, err)
      }),
    ),
  )
}
