import type { Project, Prerequisites, RepoDTO, WorkspaceDTO } from './types'
import { useChaosStore } from '@/lib/store/chaos'

const crowbar = (window as unknown as { __CROWBAR__?: { api?: string } }).__CROWBAR__
export const API_BASE: string = crowbar?.api ?? import.meta.env.VITE_API_URL ?? ''

/** Error thrown by apiFetch carrying the HTTP status, so callers can make
 *  status-specific decisions (e.g. a 404 is terminal — never retried). */
export class ApiError extends Error {
  readonly status: number
  constructor(message: string, status: number) {
    super(message)
    this.name = 'ApiError'
    this.status = status
  }
}

export function isNotFoundError(err: unknown): boolean {
  return err instanceof ApiError && err.status === 404
}

/** Tunable transient-retry policy for {@link apiFetch}. A `fetch()` *rejection*
 *  (a network/transport error — never an HTTP status) means the request never
 *  reached the daemon; at app launch that is the sidecar's unix socket not yet
 *  accepting connections. Idempotent reads retry across that startup window so a
 *  cold start lands on the workspace instead of crashing or showing an empty
 *  state. Injectable so tests run without real backoff sleeps. */
export interface RetryConfig {
  /** Total attempts including the first (1 = no retry). */
  attempts: number
  /** Exponential backoff base; delay = baseDelayMs * 2^(attempt-1), capped. */
  baseDelayMs: number
  maxDelayMs: number
  sleep?: (ms: number) => Promise<void>
}

// ~5s of bounded backoff (0,100,200,400,800,1000,1000,1000) comfortably covers a
// daemon sidecar's socket-bind window without masking a genuinely-down daemon.
const DEFAULT_RETRY: RetryConfig = { attempts: 8, baseDelayMs: 100, maxDelayMs: 1000 }

const defaultSleep = (ms: number): Promise<void> => new Promise((r) => setTimeout(r, ms))

/** Only idempotent reads auto-retry on transport failure. Mutations have their
 *  own async-202 + subscribe-before-POST resilience and must never be blindly
 *  replayed (at-least-once would double-apply). */
function isIdempotentRead(init?: RequestInit): boolean {
  const method = init?.method?.toUpperCase()
  return method === undefined || method === 'GET'
}

export async function apiFetch<T>(
  path: string,
  init?: RequestInit,
  retry: RetryConfig = DEFAULT_RETRY,
): Promise<T> {
  const { latency, errorRate, scenario, faults } = useChaosStore.getState()
  const chaosHeaders: Record<string, string> = {}
  if (latency > 0) chaosHeaders['X-Crowbar-Latency'] = String(latency)
  if (errorRate > 0) chaosHeaders['X-Crowbar-Error-Rate'] = String(errorRate)

  if (import.meta.env.VITE_USE_MOCK === 'true') {
    chaosHeaders['X-Crowbar-Scenario'] = scenario
    const activeFaults = Object.entries(faults).filter(([, v]) => v > 0)
    if (activeFaults.length > 0) {
      chaosHeaders['X-Crowbar-Fault'] = JSON.stringify(Object.fromEntries(activeFaults))
    }
  }

  const maxAttempts = isIdempotentRead(init) ? Math.max(1, retry.attempts) : 1
  const sleep = retry.sleep ?? defaultSleep

  for (let attempt = 1; ; attempt++) {
    let res: Response
    try {
      res = await fetch(`${API_BASE}${path}`, {
        ...init,
        headers: { ...init?.headers, ...chaosHeaders },
      })
    } catch (err) {
      // Transport-level failure — the request never produced an HTTP response
      // (daemon not ready / connection refused). Retry idempotent reads with
      // bounded backoff; surface the error once attempts are exhausted.
      if (attempt >= maxAttempts) throw err
      await sleep(Math.min(retry.maxDelayMs, retry.baseDelayMs * 2 ** (attempt - 1)))
      continue
    }

    const body = await res.json().catch(() => null)
    // Success with an empty/204/202 body (e.g. WriteMutationOK with no payload, a
    // 204 No Content, or a 202 Accepted for an async hierarchical mutation): the
    // envelope check below would wrongly throw, so treat it as success returning
    // undefined.
    if (res.ok && (res.status === 204 || res.status === 202 || body === null)) {
      return undefined as T
    }
    // An HTTP error status is a real daemon response, not a transport failure —
    // it is terminal (a 404 is meaningful; a 500 is a genuine server error) and
    // must never be retried.
    if (!res.ok || !body?.success) {
      throw new ApiError(body?.error ?? `${res.status} ${res.statusText}`, res.status)
    }
    return body.data as T
  }
}

export function fetchProjects(): Promise<Project[]> {
  return apiFetch('/v0/projects')
}

export function fetchProject(id: string): Promise<Project> {
  return apiFetch(`/v0/projects/${id}`)
}

// ---------------------------------------------------------------------------
// Hierarchical READ API (§3/§7).
// ---------------------------------------------------------------------------

export function fetchRepos(projectId: string): Promise<RepoDTO[]> {
  return apiFetch(`/v0/projects/${projectId}/repos`)
}

export function fetchWorkspaces(projectId: string, repoId: string): Promise<WorkspaceDTO[]> {
  return apiFetch(`/v0/projects/${projectId}/repos/${repoId}/workspaces`)
}

export function fetchWorkspace(
  projectId: string,
  repoId: string,
  wsId: string,
): Promise<WorkspaceDTO> {
  return apiFetch(`/v0/projects/${projectId}/repos/${repoId}/workspaces/${wsId}`)
}

// ---------------------------------------------------------------------------
// Hierarchical WRITE API (§3/§7) — every mutation is fire-and-forget: the
// daemon answers 202 Accepted with an empty body and the real entity (with its
// status transitions) arrives over the scoped WS broadcaster. Callers therefore
// await the WS DTO for navigation, never an id from these calls.
// ---------------------------------------------------------------------------

export function postProject(name: string, path: string, quick?: boolean): Promise<void> {
  return apiFetch('/v0/projects', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name, path, ...(quick ? { quick: true } : {}) }),
  })
}

export function postRepo(projectId: string, name: string, path: string): Promise<void> {
  return apiFetch(`/v0/projects/${projectId}/repos`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name, path }),
  })
}

// parentId omitted/empty = fork from the repo's default branch.
export function postWorkspace(
  projectId: string,
  repoId: string,
  branch: string,
  parentId?: string,
): Promise<void> {
  return apiFetch(`/v0/projects/${projectId}/repos/${repoId}/workspaces`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ branch, ...(parentId ? { parentId } : {}) }),
  })
}

export function deleteWorkspace(projectId: string, repoId: string, wsId: string): Promise<void> {
  return apiFetch(`/v0/projects/${projectId}/repos/${repoId}/workspaces/${wsId}`, {
    method: 'DELETE',
  })
}

export function fetchPrerequisites(): Promise<Prerequisites> {
  return apiFetch('/v0/system/prerequisites')
}
