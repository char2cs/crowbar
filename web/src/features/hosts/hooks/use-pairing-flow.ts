import { useRef, useState } from 'react'
import { probeRemoteFingerprint } from '@/lib/crowbar-bridge'
import { normalizeEndpoint } from '@/features/hosts/lib/endpoint'
import {
  alreadyPairedReason,
  pairingFailureText,
  redeemPairingCode,
} from '@/features/hosts/lib/pairing'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'

type PairingStage = 'address' | 'fingerprint' | 'code'

/** The host's answer the flow is waiting on; nothing may dismiss it meanwhile. */
export type PairingPending = 'probe' | 'pair' | null

/** How a code submission ended, for a form that moves focus by it. */
type CodeOutcome = 'busy' | 'invalid' | 'failed' | 'paired'

export const CODE_SHAPE_NOTICE = 'A pairing code is exactly 6 digits.'

export interface PairingFlow {
  stage: PairingStage
  name: string
  address: string
  fingerprint: string
  code: string
  /** The code was turned away for not being exactly 6 digits; the next change to it ends that. */
  turnedAway: boolean
  error: string | null
  pending: PairingPending
  setName: (name: string) => void
  setAddress: (address: string) => void
  setCode: (code: string) => void
  /** Keeps an over-long entry whole, as no code, and says why. */
  refuseCode: (code: string) => void
  /** Fields given here (a native form holds its own drafts) replace the typed ones. */
  submitAddress: (fields?: { name: string; address: string }) => Promise<void>
  confirmFingerprint: () => void
  back: () => void
  submitCode: (code?: string) => Promise<CodeOutcome>
}

interface PairingFlowOptions {
  onPaired: (hostId: string) => void
  /** Held by the owner that must refuse to be dismissed while the host is answering. */
  pending: PairingPending
  onPendingChange: (pending: PairingPending) => void
}

// Lowercase letters, digits and hyphens only, per register_remote_host's accepted hostId shape;
// a random suffix keeps two hosts named the same from colliding.
function makeHostId(name: string): string {
  const slug = name
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
  return `${slug || 'host'}-${crypto.randomUUID().slice(0, 8)}`
}

/** Pairing as stages: probe the certificate, have the user confirm the fingerprint, then redeem a
 *  code from the host's `crowbar pair`. The code stage is reached only by that confirm, and pins
 *  exactly the fingerprint shown, never a re-probed one. Its state lives as long as its caller:
 *  mount one per attempt, so a confirmed fingerprint never reaches the next. */
export function usePairingFlow({
  onPaired,
  pending,
  onPendingChange: setPending,
}: PairingFlowOptions): PairingFlow {
  const [stage, setStage] = useState<PairingStage>('address')
  const [name, setName] = useState('')
  const [address, setAddress] = useState('')
  const [fingerprint, setFingerprint] = useState('')
  const [code, setCodeValue] = useState('')
  const [turnedAway, setTurnedAway] = useState(false)
  const [error, setError] = useState<string | null>(null)
  // Minted with the probe, so every code tried after one confirm pairs the same host id.
  const hostId = useRef('')
  // The canonical address that probe dialed: the code is redeemed at, and saved as, exactly it.
  const dialed = useRef('')

  async function submitAddress(fields = { name, address }) {
    setName(fields.name)
    setAddress(fields.address)
    const trimmedName = fields.name.trim()
    if (pending || !trimmedName || !fields.address.trim()) return
    const normalized = normalizeEndpoint(fields.address)
    if ('error' in normalized) {
      setError(normalized.error)
      return
    }
    const target = normalized.endpoint
    setError(null)
    setPending('probe')
    try {
      const fp = await probeRemoteFingerprint(target)
      const paired = alreadyPairedReason(useHostConfigStore.getState().hosts, target, fp)
      if (paired) {
        setError(paired)
        return
      }
      hostId.current = makeHostId(trimmedName)
      dialed.current = target
      setFingerprint(fp)
      setStage('fingerprint')
    } catch (err) {
      setError(pairingFailureText(err, target))
    } finally {
      setPending(null)
    }
  }

  function confirmFingerprint() {
    setError(null)
    setStage('code')
  }

  function back() {
    if (pending) return
    setStage((current) => (current === 'code' ? 'fingerprint' : 'address'))
  }

  // A failure about the old code is stale the moment the code changes.
  function setCode(next: string) {
    setCodeValue(next)
    setError(null)
    setTurnedAway(false)
  }

  function refuseCode(next: string) {
    setCodeValue(next)
    setError(null)
    setTurnedAway(true)
  }

  async function submitCode(entered = code): Promise<CodeOutcome> {
    if (pending) return 'busy'
    if (entered !== code) setCode(entered)
    if (!/^\d{6}$/.test(entered)) {
      setTurnedAway(true)
      return 'invalid'
    }
    setError(null)
    setPending('pair')
    try {
      await redeemPairingCode(hostId.current, dialed.current, fingerprint, entered, name.trim())
      onPaired(hostId.current)
      return 'paired'
    } catch (err) {
      setError(pairingFailureText(err, dialed.current))
      return 'failed'
    } finally {
      setPending(null)
    }
  }

  return {
    stage,
    name,
    address,
    fingerprint,
    code,
    turnedAway,
    error,
    pending,
    setName,
    setAddress,
    setCode,
    refuseCode,
    submitAddress,
    confirmFingerprint,
    back,
    submitCode,
  }
}
