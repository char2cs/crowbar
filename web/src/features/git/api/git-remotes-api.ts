import { apiFetch, ApiError } from '@/lib/api'
import { workspaceBase } from '@/lib/workspace-scope-url'
import type { GitRemote } from '../types/git-types'

// Frozen contract with the daemon: a pull that can't fast-forward is refused
// synchronously with HTTP 409 and this exact envelope error string (no merge is
// started, the tree stays clean). Callers surface an inform-only modal on this
// code rather than the generic error path.
const NOT_FAST_FORWARDABLE = 'not_fast_forwardable'

// Remote-management (list/add/remove) has no daemon endpoint yet — still a stub.
// FUTURE: migrate to Go API calls alongside the dedicated remote-manager UI.
const tauriInvoke = async <T>(_cmd: string, _args?: unknown): Promise<T> => {
  throw new Error(`Not implemented: ${_cmd}`)
}

export interface GitRemoteActionResult {
  success: boolean
  error?: string
  /** Machine-readable refusal code from the daemon. `not_fast_forwardable` means
   *  the branch has diverged from its upstream and a pull can't fast-forward;
   *  callers open the inform-only pull-conflict modal instead of a toast. */
  code?: typeof NOT_FAST_FORWARDABLE
}

export const getRemotes = async (repoPath: string): Promise<GitRemote[]> => {
  try {
    const remotes = await tauriInvoke<GitRemote[]>('git_get_remotes', {
      repoPath,
    })
    return remotes
  } catch (error) {
    console.error('Failed to get remotes:', error)
    return []
  }
}

export const addRemote = async (repoPath: string, name: string, url: string): Promise<boolean> => {
  try {
    await tauriInvoke('git_add_remote', { repoPath, name, url })
    return true
  } catch (error) {
    console.error('Failed to add remote:', error)
    return false
  }
}

export const removeRemote = async (repoPath: string, name: string): Promise<boolean> => {
  try {
    await tauriInvoke('git_remove_remote', { repoPath, name })
    return true
  } catch (error) {
    console.error('Failed to remove remote:', error)
    return false
  }
}

// Push/pull/fetch are slow git ops: the daemon accepts them (202 Accepted) and
// runs them in the background. The real outcome — new ahead/behind counts, a
// merge conflict — arrives over the git-status WebSocket stream, not this
// response. A rejected POST (4xx/5xx) means the op never started.
const gitRemoteOp = async (
  wsId: string,
  action: 'push' | 'pull' | 'fetch',
): Promise<GitRemoteActionResult> => {
  try {
    await apiFetch(`${workspaceBase(wsId)}/git/${action}`, { method: 'POST' })
    return { success: true }
  } catch (error) {
    console.error(`Failed to ${action}:`, error)
    // The daemon refuses a non-fast-forwardable pull with 409 + the exact error
    // string `not_fast_forwardable` (apiFetch carries the status on ApiError and
    // the envelope error as its message). Surface it as a code so the caller can
    // show the inform-only modal instead of a generic error.
    if (
      error instanceof ApiError &&
      error.status === 409 &&
      error.message === NOT_FAST_FORWARDABLE
    ) {
      return { success: false, code: NOT_FAST_FORWARDABLE }
    }
    return { success: false, error: error instanceof Error ? error.message : String(error) }
  }
}

export const pushChanges = (wsId: string): Promise<GitRemoteActionResult> =>
  gitRemoteOp(wsId, 'push')

export const pullChanges = (wsId: string): Promise<GitRemoteActionResult> =>
  gitRemoteOp(wsId, 'pull')

export const fetchChanges = (wsId: string): Promise<GitRemoteActionResult> =>
  gitRemoteOp(wsId, 'fetch')
