import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import type { PairedHost } from '@/sdk/paired-host'
import type { PaneClient } from '@/sdk/pane-client'

const adoptedFromShell = ({ id, name, endpoint }: PairedHost) => ({
  id,
  name,
  endpoint,
  pairedFingerprint: '',
  clientId: '',
})

/** Makes the hosts the shell paired the hosts this pane reaches, on mount and on every change; returns the stop. */
export function followShellHosts(client: Pick<PaneClient, 'onHosts'>): () => void {
  return client.onHosts((hosts) =>
    useHostConfigStore.getState().adoptHosts(hosts.map(adoptedFromShell)),
  )
}
