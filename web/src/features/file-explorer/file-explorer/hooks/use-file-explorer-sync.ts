import { useEffect, useMemo } from "react";
import { useStore } from "zustand";
import { useWorkspaceStore } from "@/features/workspace/stores/workspace-context";
import { getExplorerTargetPath } from "@/features/file-explorer/utils/file-explorer-tree-utils";

interface UseFileExplorerSyncOptions {
  activePath?: string;
  updateActivePath?: (path: string) => void;
  revealPathInTree: (path: string) => void | Promise<void>;
}

export function useFileExplorerSync({
  activePath,
  updateActivePath,
  revealPathInTree,
}: UseFileExplorerSyncOptions) {
  const workspaceStore = useWorkspaceStore();
  const buffers = useStore(workspaceStore, (s) => s.buffers);
  const activeBufferId = useStore(workspaceStore, (s) => s.paneActions.getActivePane()?.activeBufferId ?? null);

  const activeBuffer = useMemo(
    () => buffers.find((buffer) => buffer.id === activeBufferId) || null,
    [buffers, activeBufferId],
  );

  const explorerTargetPath = useMemo(() => getExplorerTargetPath(activeBuffer), [activeBuffer]);

  useEffect(() => {
    if (!explorerTargetPath) {
      if (activePath) {
        updateActivePath?.("");
      }
      return;
    }

    if (explorerTargetPath === activePath) return;
    updateActivePath?.(explorerTargetPath);
  }, [activePath, explorerTargetPath, updateActivePath]);

  useEffect(() => {
    if (!explorerTargetPath) return;
    void revealPathInTree(explorerTargetPath);
  }, [explorerTargetPath, revealPathInTree]);
}
