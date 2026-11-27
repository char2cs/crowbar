import { useRef, useState } from 'react'
import { probeRemoteFingerprint } from '@/lib/crowbar-bridge'
import { normalizeEndpoint } from '@/features/hosts/lib/endpoint'
import {
  alreadyPairedReason,
  pairingFailureText,
  redeemPairingCode,
} from '@/features/hosts/lib/pairing'
import { localDaemonFingerprint } from '@/features/hosts/lib/device-access'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'

type PairingStage = 'address' | 'fingerprint' | 'code'

export type PairingPending = 'probe' | 'pair' | null

type CodeOutcome = 'busy' | 'invalid' | 'failed' | 'paired'

export const CODE_SHAPE_NOTICE = 'A pairing code is exactly 6 digits.'

export interface PairingFlow {
  stage: PairingStage
  name: string
  address: string
  fingerprint: string
  code: string
  turnedAway: boolean
  error: string | null
  pending: PairingPending
  setName: (name: string) => void
  setAddress: (address: string) => void
  setCode: (code: string) => void
  refuseCode: (code: string) => void
  submitAddress: (fields?: { name: string; address: string }) => Promise<void>
  confirmFingerprint: () => void
  back: () => void
  submitCode: (code?: string) => Promise<CodeOutcome>
}

interface PairingFlowOptions {
  onPaired: (hostId: string) => void
  pending: PairingPending
  onPendingChange: (pending: PairingPending) => void
}

function makeHostId(name: string): string {
  const slug = name
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
  return `${slug || 'host'}-${crypto.randomUUID().slice(0, 8)}`
}

export function usePairingFlow({
  onPaired,
  pending,
  onPendingChange: setPending,
}: PairingFlowOptions): PairingFlow {
  const [stage, setStage] = useState<PairingStage>('address')
  const [name, setName] = useState('')
  const [address, setAddressValue] = useState('')
  const [fingerprint, setFingerprint] = useState('')
  const [code, setCodeValue] = useState('')
  const [turnedAway, setTurnedAway] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const hostId = useRef('')
  const dialed = useRef('')

  function setAddress(next: string) {
    setAddressValue(next)
    if (next !== address) setError(null)
  }

  async function submitAddress(fields = { name, address }) {
    setName(fields.name)
    setAddressValue(fields.address)
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
      const paired = alreadyPairedReason(
        useHostConfigStore.getState().hosts,
        target,
        fp,
        await localDaemonFingerprint(),
      )
      if (paired) {
        setError(paired)
        return
      }
      hostId.current = makeHostId(trimmedName)
      dialed.current = target
      setFingerprint(fp)
      setStage('fingerprint')
    } catch (err) {
      setError(pairingFailureText(err, target, trimmedName))
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
    if (stage !== 'code') return 'invalid'
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
      setError(pairingFailureText(err, dialed.current, name.trim()))
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
