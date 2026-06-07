import { apiFetch } from '@/lib/api'
import type { GitCommit } from "../types/git-types";

export const commitChanges = async (_repoPath: string, _message: string): Promise<boolean> => {
  // Write operations not yet implemented in web mode
  return false;
};

export const getGitLog = async (repoPath: string, limit = 50, skip = 0): Promise<GitCommit[]> => {
  try {
    return await apiFetch<GitCommit[]>(
      `/v0/git/log?repo=${encodeURIComponent(repoPath)}&limit=${limit}&skip=${skip}`
    )
  } catch {
    return []
  }
};
