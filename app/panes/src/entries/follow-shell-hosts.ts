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

export function followShellHosts(client: Pick<PaneClient, 'onHosts'>): () => void {
  return client.onHosts((hosts) =>
    useHostConfigStore.getState().adoptHosts(hosts.map(adoptedFromShell)),
  )
}
