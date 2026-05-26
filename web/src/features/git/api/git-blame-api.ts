// Crowbar stub — FUTURE: replace with Go API calls
const tauriInvoke = async <T>(_cmd: string, _args?: unknown): Promise<T> => { throw new Error(`Not implemented: ${_cmd}`) }
import type { GitBlame } from "../types/git-types";
import { isNotGitRepositoryError, resolveRepositoryForFile } from "./git-repo-api";

export const getGitBlame = async (rootPath: string, filePath: string): Promise<GitBlame | null> => {
  try {
    const resolved = await resolveRepositoryForFile(rootPath, filePath);
    if (!resolved) {
      return null;
    }

    const blame = await tauriInvoke<GitBlame>("git_blame_file", {
      rootPath: resolved.repoPath,
      filePath: resolved.filePath,
    });
    return blame;
  } catch (error) {
    if (!isNotGitRepositoryError(error)) {
      console.error("Failed to get git blame:", error);
    }
    return null;
  }
};
