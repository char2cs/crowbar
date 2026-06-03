import { useCallback, useEffect } from "react";
import { useFileSystemStore } from "@/features/file-system/controllers/store";
import { useGitBlameStore } from "../stores/git-blame-store";
import type { GitBlameLine } from "../types/git-types";
import { dataOf } from "@/lib/loadable";

export function useGitBlame(filePath: string | undefined) {
  const rootFolderPath = useFileSystemStore((state) => state.rootFolderPath);
  const loadBlameForFile = useGitBlameStore((state) => state.loadBlameForFile);
  const blameData = useGitBlameStore((state) =>
    filePath ? dataOf(state.blame.get(filePath)) : undefined,
  );

  useEffect(() => {
    if (filePath && rootFolderPath) {
      loadBlameForFile(rootFolderPath, filePath);
    }
  }, [filePath, rootFolderPath, loadBlameForFile]);

  const getBlameForLine = useCallback(
    (lineNumber: number): GitBlameLine | null => {
      if (!filePath || !blameData) return null;

      const currentLine = lineNumber + 1;
      const blameLine = blameData.lines.find((line) => {
        const hunkStart = line.line_number;
        const hunkEnd = line.line_number + line.total_lines - 1;
        return currentLine >= hunkStart && currentLine <= hunkEnd;
      });
      return blameLine || null;
    },
    [filePath, blameData],
  );

  return { getBlameForLine };
}
