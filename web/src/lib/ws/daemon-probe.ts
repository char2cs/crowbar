import { apiBaseFor, apiFetch } from '@/lib/api/fetch'
import { isTauri } from '@/lib/crowbar-bridge'
import { useDaemonHaltStore } from '@/features/window/stores/daemon-halt-store'
import { toast } from '@/features/window/stores/toast-store'
import { LOCAL_HOST } from '@/features/hosts/types'
import { reportMigrating, type Migrating } from './connection-store'

type DaemonHealthState =
  { kind: 'migrating'; migrating: Migrating } | { kind: 'halted'; message: string } | null

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null
}

// The health route carries no message, only detail; these are the daemon's own
// words for the same states (bootreport.go).
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

/** The state a health read's `data` reports, or null for a daemon that is serving. */
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

// One attempt: the socket that failed is already retrying, and each failed
// retry asks again.
const ONCE = { attempts: 1, baseDelayMs: 0, maxDelayMs: 0 }
// An explicit init opts out of joining a read in flight, which may be stale or hung.
const UNSHARED: RequestInit = { method: 'GET' }

// Sockets opened so far: an answer to a probe sent before the latest open
// describes a daemon that has since served, so it is dropped.
let opened = 0
let inFlight: { sentAt: number; read: Promise<void> } | null = null

/** Called by the wsManager each time a local socket opens; `resumed` when it
 *  replaces one that dropped, which is when the daemon may have been replaced. */
export function noteSocketOpened(resumed = false): void {
  opened++
  if (resumed || !buildChecked) void checkBuild()
}

let buildChecked = false
let buildRead: Promise<void> | null = null

// The page carries its commit's short form, the daemon the full one. A dev build,
// or an older daemon that names none, has nothing to compare.
function sameBuild(page: string, daemon: string): boolean {
  if (!page || !daemon || page === 'dev' || daemon === 'dev') return true
  return daemon.startsWith(page) || page.startsWith(daemon)
}

/** A page the daemon served compares its build with the daemon's. One of another
 *  build is offered a reload, never given one: an unsent prompt lives only in its
 *  composer. The desktop shell ships its page with its daemon and checks it itself. */
function checkBuild(): Promise<void> {
  if (import.meta.env.DEV || isTauri() || apiBaseFor(LOCAL_HOST.id) !== '') return Promise.resolve()
  buildChecked = true
  buildRead ??= apiFetch<unknown>('/v0/health', UNSHARED, ONCE)
    .then((data) => {
      const daemon = isRecord(data) && typeof data.build === 'string' ? data.build : ''
      if (sameBuild(__APP_VERSION__, daemon)) return
      toast.show({
        key: 'daemon-build',
        type: 'info',
        duration: 0,
        message: 'Crowbar was updated',
        description: 'Reload to use the new version. Text you have not sent is lost on reload.',
        action: { label: 'Reload', onClick: () => window.location.reload() },
      })
    })
    .catch((err: unknown) => {
      if (import.meta.env.DEV) console.warn('daemon build check failed:', err)
    })
    .finally(() => {
      buildRead = null
    })
  return buildRead
}

async function readHealth(sentAt: number): Promise<void> {
  const state = daemonStateFromHealth(await apiFetch<unknown>('/v0/health', UNSHARED, ONCE))
  if (sentAt !== opened) return
  if (state?.kind === 'migrating') reportMigrating(state.migrating)
  if (state?.kind === 'halted') useDaemonHaltStore.getState().halt(state.message)
}

/** Asks the daemon why a socket was refused. A read that fails says nothing
 *  new, so it leaves the state as it is. A read that never answers holds only
 *  probes of its own generation: the next socket open starts a fresh one. */
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
