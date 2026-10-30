import { pairRemoteHost } from '@/lib/crowbar-bridge'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import type { Host } from '@/features/hosts/types'

/** Pairs `hostId` with the fingerprint the user confirmed: exactly what they saw, never one
 *  re-probed or re-derived here. */
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

/** Why a host at `endpoint` presenting `fingerprint` can't be paired: a configured host is it
 *  already. Null when none is. `endpoint` is canonical (`normalizeEndpoint`), as saved ones are. */
export function alreadyPairedReason(
  hosts: readonly Host[],
  endpoint: string,
  fingerprint: string,
): string | null {
  const remedy = 'Remove it in Settings → Hosts to pair it again.'
  const atAddress = hosts.find((host) => host.endpoint === endpoint)
  if (atAddress) return `This address is already paired as ${atAddress.name}. ${remedy}`
  const same = hosts.find((host) => fingerprint !== '' && host.pairedFingerprint === fingerprint)
  return same ? `This host is already paired as ${same.name}. ${remedy}` : null
}

const REFUSED = /^the host refused pairing \((\d{3})[^)]*\): (.*)$/s
const UNRESOLVED = /^cannot resolve ([^\s:]+)/
const UNREACHABLE = /^cannot reach |did not answer within|did not finish pairing|^pairing with /
const NOT_CROWBAR = /^TLS handshake with |presented no certificate/

/** What a failed probe or pairing of the host `name` at `address` means for the user, in plain
 *  words: the desktop shell's own text names transport and OS errors, which are never shown. */
export function pairingFailureText(err: unknown, address: string, name: string): string {
  const text = err instanceof Error ? err.message : String(err)
  const refused = REFUSED.exec(text)
  // A wrong code and an expired one get the same 401: the host does not say which.
  if (refused?.[1] === '401') {
    return 'That code didn’t work. It may be mistyped or expired — the host doesn’t say which. Check it, or run crowbar pair on the host again for a new one.'
  }
  if (refused?.[1] === '429') {
    return 'Too many attempts — wait a minute, or run crowbar pair on the host for a new code.'
  }
  if (refused) return `${address} refused pairing: ${refused[2]}.`
  if (text.includes('identity changed')) {
    return `The host at ${address} no longer presents the fingerprint you confirmed, so nothing was sent to it. Go back and check the address.`
  }
  const unresolved = UNRESOLVED.exec(text)
  if (unresolved) {
    return `Couldn’t find ${unresolved[1]}. Check the spelling, or use its IP address.`
  }
  if (UNREACHABLE.test(text)) {
    return `Can’t reach ${address}. Check the address and port, and that the host is running.`
  }
  if (NOT_CROWBAR.test(text)) {
    return `${address} answered, but not as a Crowbar host. Check the address and port.`
  }
  return `Couldn’t pair with ${name}. Try again, or run crowbar pair on the host for a new code.`
}
