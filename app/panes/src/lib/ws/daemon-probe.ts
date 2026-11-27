import { apiFetch } from '@/lib/api/fetch'
import { useDaemonHaltStore } from '@/features/window/stores/daemon-halt-store'
import { reportMigrating, type Migrating } from './connection-store'

type DaemonHealthState =
  { kind: 'migrating'; migrating: Migrating } | { kind: 'halted'; message: string } | null

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null
}

function migratingFrom(detail: Record<string, unknown>): Migrating {
  const { index, total, step } = detail
  if (typeof index !== 'number' || typeof total !== 'number' || !isRecord(step)) {
    return { message: 'Crowbar is starting' }
  }
  const name =
    typeof step.name === 'string' && step.name ? step.name : `step ${String(step.number)}`
  return {
    message: `Crowbar is upgrading its data (step ${index} of ${total}: ${name})`,
    step: { index, total, name },
  }
}

function daemonStateFromHealth(data: unknown): DaemonHealthState {
  if (!isRecord(data)) return null
  const detail = isRecord(data.detail) ? data.detail : {}
  if (data.status === 'halted') {
    const message = typeof detail.message === 'string' ? detail.message : ''
    return { kind: 'halted', message: message || 'the daemon halted without saying why' }
  }
  if (data.status === 'migrating') return { kind: 'migrating', migrating: migratingFrom(detail) }
  return null
}

const ONCE = { attempts: 1, baseDelayMs: 0, maxDelayMs: 0 }
const UNSHARED: RequestInit = { method: 'GET' }

let opened = 0
let inFlight: { sentAt: number; read: Promise<void> } | null = null

export function noteSocketOpened(): void {
  opened++
}

async function readHealth(sentAt: number): Promise<void> {
  const state = daemonStateFromHealth(await apiFetch<unknown>('/v0/health', UNSHARED, ONCE))
  if (sentAt !== opened) return
  if (state?.kind === 'migrating') reportMigrating(state.migrating)
  if (state?.kind === 'halted') useDaemonHaltStore.getState().halt(state.message)
}

export function probeDaemon(): Promise<void> {
  if (inFlight?.sentAt === opened) return inFlight.read
  const sentAt = opened
  const read: Promise<void> = readHealth(sentAt)
    .catch((err: unknown) => {
      if (import.meta.env.DEV) console.warn('daemon health probe failed:', err)
    })
    .finally(() => {
      if (inFlight?.read === read) inFlight = null
    })
  inFlight = { sentAt, read }
  return read
}
