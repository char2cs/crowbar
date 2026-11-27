import { apiFetch } from '@/lib/api/fetch'
import { classifyHostFailure } from '@/features/hosts/lib/classify-host-failure'

const HOST_PROBE_TIMEOUT_MS = 5_000

const ONCE = { attempts: 1, baseDelayMs: 0, maxDelayMs: 0 }

type HostProbe = { status: 'up' | 'down' | 'needs-repair' }

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
