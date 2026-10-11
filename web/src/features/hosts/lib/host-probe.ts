import { apiFetch } from '@/lib/api'
import { classifyHostFailure } from '@/features/hosts/lib/classify-host-failure'

// A host that accepts the connection and never answers would otherwise hold the probe
// for the proxy's five-minute request timeout, its status frozen at the last answer.
const HOST_PROBE_TIMEOUT_MS = 5_000

// The next probe is the retry.
const ONCE = { attempts: 1, baseDelayMs: 0, maxDelayMs: 0 }

type HostProbe = { status: 'up' | 'down' | 'needs-repair' }

/** Reads one host's /v0/health once and says what that makes the host. */
export async function probeHost(hostId: string): Promise<HostProbe> {
  const abort = new AbortController()
  const timer = setTimeout(() => abort.abort(), HOST_PROBE_TIMEOUT_MS)
  try {
    await apiFetch<void>('/v0/health', { signal: abort.signal }, ONCE, hostId)
    return { status: 'up' }
  } catch (err) {
    return { status: classifyHostFailure(err) }
  } finally {
    clearTimeout(timer)
  }
}
