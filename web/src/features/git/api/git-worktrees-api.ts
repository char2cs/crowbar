// Crowbar stub — FUTURE: replace with Go API calls
const tauriInvoke = async <T>(_cmd: string, _args?: unknown): Promise<T> => {
  throw new Error(`Not implemented: ${_cmd}`)
}

export const removeWorktree = async (
  repoPath: string,
  path: string,
  force: boolean = false,
): Promise<boolean> => {
  try {
    await tauriInvoke('git_remove_worktree', { repoPath, path, force })
    return true
  } catch (error) {
    console.error('Failed to remove worktree:', error)
    return false
  }
}
