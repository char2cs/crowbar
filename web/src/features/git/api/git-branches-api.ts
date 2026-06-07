import { apiFetch } from '@/lib/api'
import type { Branch } from '@/lib/mock/git-data'

// Crowbar stub — write operations not yet implemented in web mode
const tauriInvoke = async <T>(_cmd: string, _args?: unknown): Promise<T> => { throw new Error(`Not implemented: ${_cmd}`) }

interface CheckoutResult {
  success: boolean;
  hasChanges: boolean;
  message: string;
}

export const getBranches = async (repoPath: string): Promise<string[]> => {
  try {
    const branches = await apiFetch<Branch[]>(
      `/v0/git/branches?repo=${encodeURIComponent(repoPath)}`
    )
    return branches.map((b) => b.name)
  } catch {
    return [];
  }
};

export const checkoutBranch = async (
  repoPath: string,
  branchName: string,
): Promise<CheckoutResult> => {
  try {
    const result = await tauriInvoke<CheckoutResult>("git_checkout", {
      repoPath,
      branchName,
    });
    return result;
  } catch (error) {
    console.error("Failed to checkout branch:", error);
    return {
      success: false,
      hasChanges: false,
      message: "Failed to checkout branch",
    };
  }
};

export const createBranch = async (
  repoPath: string,
  branchName: string,
  fromBranch?: string,
): Promise<boolean> => {
  try {
    await tauriInvoke("git_create_branch", {
      repoPath,
      branchName,
      fromBranch,
    });
    return true;
  } catch (error) {
    console.error("Failed to create branch:", error);
    return false;
  }
};

export const deleteBranch = async (repoPath: string, branchName: string): Promise<boolean> => {
  try {
    await tauriInvoke("git_delete_branch", { repoPath, branchName });
    return true;
  } catch (error) {
    console.error("Failed to delete branch:", error);
    return false;
  }
};
