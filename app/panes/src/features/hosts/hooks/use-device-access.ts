import { useCallback, useEffect, useState } from 'react'
import {
  getDeviceAccess,
  listPairedDevices,
  mintPairingOffer,
  revokeDevice,
  setDeviceAccess,
  type DeviceAccess,
  type PairedDevice,
  type PairingOffer,
} from '@/features/hosts/lib/device-access'
import { ApiError } from '@/lib/errors/api-error'
import { userWords } from '@/lib/errors/report'

const wording = (err: unknown): string => userWords(err, String(err)) ?? 'Something went wrong.'

const NOT_THE_OWNER =
  'Device connections can only be changed on the computer this instance runs on.'

type Load = { status: 'loading' } | { status: 'failed'; why: string } | { status: 'ready' }

interface DeviceAccessView {
  load: Load
  access: DeviceAccess | null
  devices: PairedDevice[]
  setEnabled: (enabled: boolean, address: string) => Promise<string | null>
  addDevice: () => Promise<PairingOffer>
  revoke: (id: string) => Promise<string | null>
  refreshDevices: () => Promise<string | null>
  retry: () => void
}

export function useDeviceAccess(hostId: string): DeviceAccessView {
  const [load, setLoad] = useState<Load>({ status: 'loading' })
  const [access, setAccess] = useState<DeviceAccess | null>(null)
  const [devices, setDevices] = useState<PairedDevice[]>([])
  const [attempt, setAttempt] = useState(0)

  useEffect(() => {
    let current = true
    setLoad({ status: 'loading' })
    Promise.all([getDeviceAccess(hostId), listPairedDevices(hostId)]).then(
      ([nextAccess, nextDevices]) => {
        if (!current) return
        setAccess(nextAccess)
        setDevices(nextDevices)
        setLoad({ status: 'ready' })
      },
      (err: unknown) => {
        if (current) setLoad({ status: 'failed', why: wording(err) })
      },
    )
    return () => {
      current = false
    }
  }, [hostId, attempt])

  const setEnabled = useCallback(
    async (enabled: boolean, address: string) => {
      try {
        setAccess(await setDeviceAccess(hostId, enabled ? { enabled, address } : { enabled }))
        return null
      } catch (err) {
        if (err instanceof ApiError && err.status === 403) return NOT_THE_OWNER
        return wording(err)
      }
    },
    [hostId],
  )

  const revoke = useCallback(
    async (id: string) => {
      try {
        await revokeDevice(hostId, id)
        setDevices(await listPairedDevices(hostId))
        return null
      } catch (err) {
        return wording(err)
      }
    },
    [hostId],
  )

  const refreshDevices = useCallback(async () => {
    try {
      setDevices(await listPairedDevices(hostId))
      return null
    } catch (err) {
      return wording(err)
    }
  }, [hostId])

  const addDevice = useCallback(() => mintPairingOffer(hostId), [hostId])
  const retry = useCallback(() => setAttempt((n) => n + 1), [])
  return { load, access, devices, setEnabled, addDevice, revoke, refreshDevices, retry }
}
