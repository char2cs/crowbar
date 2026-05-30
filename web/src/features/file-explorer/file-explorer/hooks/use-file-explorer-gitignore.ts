import { useEffect, useState } from "react";
import { readFile } from "@/features/file-system/controllers/platform";
import {
  createFileTreeGitIgnoreRules,
  isPathGitIgnoredByFileTreeRules,
  type FileTreeGitIgnoreRules,
  type GitIgnoreFileContent,
  type GitIgnoreFileReference,
} from "@/features/file-explorer/lib/file-tree-gitignore";

interface UseFileExplorerGitignoreResult {
  isGitIgnored: (fullPath: string, isDir: boolean) => boolean;
}

export function useFileExplorerGitignore(
  rootFolderPath: string | undefined,
  gitIgnoreFileReferences: GitIgnoreFileReference[],
  getWorkspaceRootForPath: (path: string) => string | undefined,
): UseFileExplorerGitignoreResult {
  const [gitIgnoreRules, setGitIgnoreRules] = useState<FileTreeGitIgnoreRules | null>(null);

  useEffect(() => {
    let cancelled = false;

    const loadGitignore = async () => {
      if (!rootFolderPath) {
        setGitIgnoreRules(null);
        return;
      }

      const ignoreFiles = await Promise.all(
        gitIgnoreFileReferences.map(async (file): Promise<GitIgnoreFileContent | null> => {
          try {
            return {
              ...file,
              content: await readFile(file.path),
            };
          } catch {
            return null;
          }
        }),
      );

      if (!cancelled) {
        setGitIgnoreRules(
          createFileTreeGitIgnoreRules(
            rootFolderPath,
            ignoreFiles.filter((file): file is GitIgnoreFileContent => file !== null),
          ),
        );
      }
    };

    void loadGitignore();

    return () => {
      cancelled = true;
    };
  }, [gitIgnoreFileReferences, rootFolderPath]);

  const isGitIgnored = (fullPath: string, isDir: boolean): boolean => {
    if (!gitIgnoreRules || !rootFolderPath) return false;
    if (getWorkspaceRootForPath(fullPath) !== rootFolderPath) return false;
    return isPathGitIgnoredByFileTreeRules(gitIgnoreRules, fullPath, isDir);
  };

  return { isGitIgnored };
}
