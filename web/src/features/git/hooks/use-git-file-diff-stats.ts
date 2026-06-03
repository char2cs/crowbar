import { useEffect, useState } from "react";
import { getFileDiff } from "../api/git-diff-api";
import type { GitFile } from "../types/git-types";
import { countDiffStats } from "../utils/git-diff-helpers";

interface GitFileDiffStats {
  additions: number;
  deletions: number;
}

const MAX_STATUS_DIFF_STATS_FILES = 40;

export function useGitFileDiffStats(
  activeRepoPath: string | null,
  visibleGitFiles: GitFile[],
): Record<string, GitFileDiffStats> {
  const [fileDiffStats, setFileDiffStats] = useState<Record<string, GitFileDiffStats>>({});

  useEffect(() => {
    if (!activeRepoPath || !visibleGitFiles.length) {
      setFileDiffStats({});
      return;
    }

    let isCancelled = false;

    const loadFileDiffStats = async () => {
      const uniqueFiles = Array.from(
        new Map(
          visibleGitFiles.map((file) => [
            `${file.staged ? "staged" : "unstaged"}:${file.path}`,
            file,
          ]),
        ).values(),
      );
      const filesToMeasure = uniqueFiles.slice(0, MAX_STATUS_DIFF_STATS_FILES);

      const statsEntries = await Promise.all(
        filesToMeasure.map(async (file) => {
          const diff = await getFileDiff(activeRepoPath, file.path, file.staged);
          const { additions, deletions } = diff
            ? countDiffStats([diff])
            : { additions: 0, deletions: 0 };
          return [
            `${file.staged ? "staged" : "unstaged"}:${file.path}`,
            { additions, deletions },
          ] as const;
        }),
      );

      if (!isCancelled) {
        setFileDiffStats(Object.fromEntries(statsEntries));
      }
    };

    void loadFileDiffStats();

    return () => {
      isCancelled = true;
    };
  }, [activeRepoPath, visibleGitFiles]);

  return fileDiffStats;
}
