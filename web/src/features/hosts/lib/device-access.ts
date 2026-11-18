import { hasLocalDaemon } from '@/features/hosts/lib/local-daemon'
import { LOCAL_HOST } from '@/features/hosts/types'
import { apiFetch, type RetryConfig } from '@/lib/api/fetch'

/** Whether an instance accepts connections from other devices, and the identity they confirm.
 *  The fingerprint is known while it is off: it is how a client tells its own daemon apart. */
export interface DeviceAccess {
  enabled: boolean
  /** The configured `host:port` bind, kept while closed. */
  address: string
  /** Where another device dials while it is on. */
  reachable: string[]
  fingerprint: string
  error?: string
}

export interface PairedDevice {
  id: string
  name: string
  pairedAt: string
  lastSeen: string
}

/** A one-time code and what the device needs beside it. */
export interface PairingOffer {
  code: string
  serverFingerprint: string
  addresses: string[]
  expiresAt: string
}

export function getDeviceAccess(hostId: string, retry?: RetryConfig): Promise<DeviceAccess> {
  return apiFetch<DeviceAccess>('/v0/access', undefined, retry, hostId)
}

export function setDeviceAccess(
  hostId: string,
  change: { enabled: boolean; address?: string },
): Promise<DeviceAccess> {
  return apiFetch<DeviceAccess>(
    '/v0/access',
    {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(change),
    },
    undefined,
    hostId,
  )
}

export function mintPairingOffer(hostId: string): Promise<PairingOffer> {
  return apiFetch<PairingOffer>('/v0/pair/code', { method: 'POST' }, undefined, hostId)
}

export async function revokeDevice(hostId: string, clientId: string): Promise<void> {
  await apiFetch(
    `/v0/clients/${encodeURIComponent(clientId)}`,
    { method: 'DELETE' },
    undefined,
    hostId,
  )
}

interface ClientRow extends PairedDevice {
  revokedAt?: string
}

/** The devices still paired to the instance, newest first: a revoked row stays in its registry. */
export async function listPairedDevices(hostId: string): Promise<PairedDevice[]> {
  const rows = await apiFetch<ClientRow[]>('/v0/clients', undefined, undefined, hostId)
  return rows
    .filter((row) => !row.revokedAt)
    .sort((a, b) => Date.parse(b.pairedAt) - Date.parse(a.pairedAt))
}

/** The TLS identity of this computer's own daemon, which a host reached by address is told apart
 *  from; '' where there is no daemon or it cannot be asked (nothing is then taken for Local). */
export async function localDaemonFingerprint(): Promise<string> {
  if (!hasLocalDaemon()) return ''
  try {
    return (await getDeviceAccess(LOCAL_HOST.id, { attempts: 1, baseDelayMs: 0, maxDelayMs: 0 }))
      .fingerprint
  } catch {
    return ''
  }
}
