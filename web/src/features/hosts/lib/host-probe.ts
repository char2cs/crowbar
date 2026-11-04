import { apiFetch } from '@/lib/api/fetch'
import { classifyHostFailure } from '@/features/hosts/lib/classify-host-failure'

// A host that accepts the connection and never answers would otherwise hold the probe
// for the proxy's five-minute request timeout, its status frozen at the last answer.
const HOST_PROBE_TIMEOUT_MS = 5_000

// The next probe is the retry.
const ONCE = { attempts: 1, baseDelayMs: 0, maxDelayMs: 0 }

type HostProbe = { status: 'up' | 'down' | 'needs-repair' }

/** Reads one host's /v0/health once and says what that makes the host. */
export async function probeHost(hostId: string): Promise<HostProbe> {
  try {
    await apiFetch<void>(
      '/v0/health',
      { signal: AbortSignal.timeout(HOST_PROBE_TIMEOUT_MS) },
      ONCE,
      hostId,
    )
    return { status: 'up' }
  } catch (err) {
    return { status: classifyHostFailure(err) }
  }
}
