import { isTauri, registerRemoteHost } from '@/lib/crowbar-bridge'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'

let registration: Promise<void> | undefined

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
