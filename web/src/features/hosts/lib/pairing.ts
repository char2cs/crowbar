import { pairRemoteHost } from '@/lib/crowbar-bridge'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'

/**
 * Redeems a pairing code against a host the user has confirmed the
 * fingerprint of (via probeRemoteFingerprint, in the dialog's stage 1).
 * `confirmedFingerprint` must be exactly what the user saw and approved —
 * never re-derive or re-probe it here.
 */
export async function redeemPairingCode(
  hostId: string,
  endpoint: string,
  confirmedFingerprint: string,
  code: string,
  name: string,
): Promise<void> {
  const clientId = await pairRemoteHost(hostId, endpoint, confirmedFingerprint, code, name)
  useHostConfigStore.getState().addHost({
    id: hostId,
    name,
    endpoint,
    pairedFingerprint: confirmedFingerprint,
    clientId,
  })
}
