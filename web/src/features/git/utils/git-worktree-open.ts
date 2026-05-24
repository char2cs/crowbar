import { useFileSystemStore } from "@/features/file-system/controllers/store";
import { useRepositoryStore } from "@/features/git/stores/git-repository-store";
import { createAppWindow } from "@/features/window/utils/create-app-window";

type GitWorktreeOpenTarget = "current-window" | "new-window";

interface OpenGitWorktreeOptions {
  target?: GitWorktreeOpenTarget;
}

export async function openGitWorktreeWorkspace(
  worktreePath: string,
  options: OpenGitWorktreeOptions = {},
): Promise<boolean> {
  const path = worktreePath.trim();
  if (!path) return false;

  if (options.target === "new-window") {
    await createAppWindow({
      path,
      isDirectory: true,
    });
    return true;
  }

  const { handleOpenFolderByPath } = useFileSystemStore.getState();
  if (!handleOpenFolderByPath) return false;
  await handleOpenFolderByPath(path);

  useRepositoryStore.getState().actions.selectRepository(path);
  return true;
}
