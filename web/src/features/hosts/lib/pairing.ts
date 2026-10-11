import { pairRemoteHost } from '@/lib/crowbar-bridge'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import type { Host } from '@/features/hosts/types'

/**
 * Pairs `hostId` with the fingerprint the user confirmed: exactly what they saw, never one
 * re-probed or re-derived here.
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

const DEFAULT_PORT = 3737

/** The host and port `endpoint` dials, however it was typed: scheme, case, trailing slash. */
function dialTarget(endpoint: string): string {
  const bare = endpoint
    .trim()
    .replace(/^[a-z][a-z\d+.-]*:\/\//i, '')
    .replace(/\/+$/, '')
    .toLowerCase()
  return /:\d+$/.test(bare) && !bare.endsWith(']') ? bare : `${bare}:${DEFAULT_PORT}`
}

/** Why a host at `endpoint` presenting `fingerprint` can't be paired: a configured host is it
 *  already. Null when none is. */
export function alreadyPairedReason(
  hosts: readonly Host[],
  endpoint: string,
  fingerprint: string,
): string | null {
  const remedy = 'Remove it in Settings → Hosts to pair it again.'
  const target = dialTarget(endpoint)
  const atAddress = hosts.find((host) => dialTarget(host.endpoint) === target)
  if (atAddress) return `This address is already paired as ${atAddress.name}. ${remedy}`
  const same = hosts.find((host) => fingerprint !== '' && host.pairedFingerprint === fingerprint)
  return same ? `This host is already paired as ${same.name}. ${remedy}` : null
}
