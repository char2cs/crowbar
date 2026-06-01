import { create } from "zustand";
import { frontendTrace } from "@/utils/frontend-trace";
import { getGitLog } from "../api/git-commits-api";
import { getGitStatus } from "../api/git-status-api";
import { getBranches } from "../api/git-branches-api";
import { getStashes } from "../api/git-stash-api";
import type { GitCommit, GitStash, GitStatus } from "../types/git-types";
import { createLoadableSlice } from "@/lib/store/loadable-slice";
import type { Loadable } from "@/lib/loadable";

const MAX_WORKSPACE_GIT_STATUS_FILES = 200;

function toWorkspaceGitStatus(status: GitStatus | null): GitStatus | null {
  if (!status) return null;
  if (status.files.length <= MAX_WORKSPACE_GIT_STATUS_FILES) return status;

  console.info("[workspace-git] truncating workspace git status", {
    totalFiles: status.files.length,
    keptFiles: MAX_WORKSPACE_GIT_STATUS_FILES,
  });
  frontendTrace("warn", "workspace-git", "truncate", {
    totalFiles: status.files.length,
    keptFiles: MAX_WORKSPACE_GIT_STATUS_FILES,
  });

  return {
    ...status,
    files: status.files.slice(0, MAX_WORKSPACE_GIT_STATUS_FILES),
  };
}

export interface GitData {
  status: GitStatus | null;
  commits: GitCommit[];
  branches: string[];
  stashes: GitStash[];
}

async function fetchAllGitData(repoPath: string): Promise<GitData> {
  const [status, commits, branches, stashes] = await Promise.all([
    getGitStatus(repoPath),
    getGitLog(repoPath, 50, 0),
    getBranches(repoPath),
    getStashes(repoPath),
  ]);
  return { status, commits, branches, stashes };
}

interface GitState {
  gitData: Loadable<GitData>;
  fetchGitData: (repoPath: string) => Promise<void>;
  startGitSync: (repoPath: string) => () => void;
  applyGitDelta: (event: unknown, repoPath: string) => Promise<void>;
  optimisticWriteGitData: (optimistic: GitData, commit: () => Promise<GitData | void>) => Promise<void>;
  gitStatus: GitStatus | null;
  workspaceGitStatus: GitStatus | null;
  commits: GitCommit[];
  branches: string[];
  stashes: GitStash[];
  hasMoreCommits: boolean;
  isLoadingMoreCommits: boolean;
  isLoadingGitData: boolean;
  isRefreshing: boolean;
  currentRepoPath: string | null;
  currentWorkspaceRepoPath: string | null;

  actions: {
    loadFreshGitData: (data: {
      gitStatus: GitStatus | null;
      commits: GitCommit[];
      branches: string[];
      stashes: GitStash[];
      repoPath: string;
    }) => void;
    refreshGitData: (data: {
      gitStatus: GitStatus | null;
      branches: string[];
      repoPath: string;
    }) => Promise<void>;
    refreshWorkspaceGitStatus: (repoPath: string) => Promise<void>;
    loadMoreCommits: (repoPath: string) => Promise<void>;
    setGitStatus: (status: GitStatus | null) => void;
    setWorkspaceGitStatus: (status: GitStatus | null, repoPath: string | null) => void;
    setCommits: (commits: GitCommit[]) => void;
    setBranches: (branches: string[]) => void;
    setStashes: (stashes: GitStash[]) => void;
    setIsLoadingGitData: (loading: boolean) => void;
    setIsRefreshing: (refreshing: boolean) => void;
    reset: () => void;
  };
}

const COMMITS_PER_PAGE = 50;

export const useGitStore = create<GitState>((set, get) => ({
  ...(() => {
    const slice = createLoadableSlice<GitData>({
      store: "git-data",
      fetcher: fetchAllGitData,
      wsEndpoint: (repoPath: string) => `/api/v0/ws/git?repo=${encodeURIComponent(repoPath)}`,
    })(
      // setter: remap the slice's { data } writes onto the host's gitData field.
      // The slice only ever calls set({ data: ... }) (verified in loadable-slice.ts);
      // the `'data' in partial` guard makes that assumption explicit.
      (partial) => {
        if (partial && "data" in partial) {
          set({ gitData: (partial as { data: Loadable<GitData> }).data } as Partial<GitState>);
        }
      },
      // getter: expose host fields under the slice's expected shape so the slice's
      // internal get().fetch / get().applyDelta resolve to the host methods.
      // `as never` bridges the host's renamed fields to the slice's flat LoadableSlice shape.
      () =>
        ({
          data: get().gitData,
          fetch: get().fetchGitData,
          startSync: get().startGitSync,
          applyDelta: get().applyGitDelta,
          optimisticWrite: get().optimisticWriteGitData,
        }) as never,
    );
    return {
      gitData: slice.data,
      fetchGitData: slice.fetch,
      startGitSync: slice.startSync,
      applyGitDelta: slice.applyDelta,
      optimisticWriteGitData: slice.optimisticWrite,
    };
  })(),
  gitStatus: null,
  workspaceGitStatus: null,
  commits: [],
  branches: [],
  stashes: [],
  hasMoreCommits: true,
  isLoadingMoreCommits: false,
  isLoadingGitData: false,
  isRefreshing: false,
  currentRepoPath: null,
  currentWorkspaceRepoPath: null,

  actions: {
    loadFreshGitData: ({ gitStatus, commits, branches, stashes, repoPath }) => {
      set({
        gitStatus,
        // Also drive file-explorer git decorations — file-explorer-tree reads
        // workspaceGitStatus + currentWorkspaceRepoPath from this store
        workspaceGitStatus: toWorkspaceGitStatus(gitStatus),
        currentWorkspaceRepoPath: repoPath,
        commits,
        branches,
        stashes,
        hasMoreCommits: commits.length >= COMMITS_PER_PAGE,
        currentRepoPath: repoPath,
      });
    },

    refreshGitData: async ({ gitStatus, branches, repoPath }) => {
      const { currentRepoPath, commits: existingCommits } = get();

      if (currentRepoPath !== repoPath || existingCommits.length === 0) {
        set({ gitStatus, workspaceGitStatus: toWorkspaceGitStatus(gitStatus), currentWorkspaceRepoPath: repoPath, branches });
        return;
      }

      const latestCommits = await getGitLog(repoPath, 50, 0);

      if (latestCommits.length > 0) {
        const existingHashSet = new Set(existingCommits.map((c) => c.hash));
        const newCommits = latestCommits.filter((c) => !existingHashSet.has(c.hash));

        if (newCommits.length > 0) {
          set({
            gitStatus,
            branches,
            commits: [...newCommits, ...existingCommits],
          });
        } else {
          set({ gitStatus, branches });
        }
      } else {
        set({ gitStatus, branches });
      }
    },

    refreshWorkspaceGitStatus: async (repoPath) => {
      const status = await getGitStatus(repoPath);

      if (get().currentWorkspaceRepoPath !== repoPath) {
        return;
      }

      set({
        workspaceGitStatus: toWorkspaceGitStatus(status),
      });
    },

    loadMoreCommits: async (repoPath) => {
      const { commits, hasMoreCommits, isLoadingMoreCommits } = get();

      if (!hasMoreCommits || isLoadingMoreCommits) return;

      set({ isLoadingMoreCommits: true });

      try {
        const newCommits = await getGitLog(repoPath, COMMITS_PER_PAGE, commits.length);

        const existingHashSet = new Set(commits.map((c) => c.hash));
        const uniqueNewCommits = newCommits.filter((c) => !existingHashSet.has(c.hash));

        if (uniqueNewCommits.length > 0) {
          set({
            commits: [...commits, ...uniqueNewCommits],
            hasMoreCommits: uniqueNewCommits.length >= COMMITS_PER_PAGE,
          });
        } else {
          set({ hasMoreCommits: false });
        }
      } finally {
        set({ isLoadingMoreCommits: false });
      }
    },

    setGitStatus: (status) => set({ gitStatus: status }),
    setWorkspaceGitStatus: (status, repoPath) =>
      set({
        workspaceGitStatus: toWorkspaceGitStatus(status),
        currentWorkspaceRepoPath: repoPath,
      }),
    setCommits: (commits) => set({ commits }),
    setBranches: (branches) => set({ branches }),
    setStashes: (stashes) => set({ stashes }),
    setIsLoadingGitData: (loading) => set({ isLoadingGitData: loading }),
    setIsRefreshing: (refreshing) => set({ isRefreshing: refreshing }),

    reset: () =>
      set({
        gitStatus: null,
        commits: [],
        branches: [],
        stashes: [],
        hasMoreCommits: true,
        isLoadingMoreCommits: false,
        isLoadingGitData: false,
        isRefreshing: false,
        currentRepoPath: null,
        currentWorkspaceRepoPath: null,
        workspaceGitStatus: null,
      }),
  },
}));
