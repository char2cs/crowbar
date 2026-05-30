import { useMemo, useEffect } from "react";
import { useActiveWorkspaceState } from "@/features/workspace/stores/hooks/use-active-workspace-state";
import { getExplorerTargetPath } from "@/features/file-explorer/utils/file-explorer-tree-utils";
import type { PaneContent } from "@/features/panes/types/pane-content";

interface UseFileExplorerSyncOptions {
  activePath?: string;
  updateActivePath?: (path: string) => void;
  revealPathInTree: (path: string) => void | Promise<void>;
}

const EMPTY_BUFFERS: PaneContent[] = [];

const selectBuffers = (s: { buffers: PaneContent[] }) => s.buffers;
const selectActiveBufferId = (s: {
  paneActions: { getActivePane(): { activeBufferId: string | null } | null | undefined };
}) => s.paneActions.getActivePane()?.activeBufferId ?? null;

export function useFileExplorerSync({
  activePath,
  updateActivePath,
  revealPathInTree,
}: UseFileExplorerSyncOptions) {
  const buffers = useActiveWorkspaceState(selectBuffers, EMPTY_BUFFERS);
  const activeBufferId = useActiveWorkspaceState(selectActiveBufferId, null);

  const activeBuffer = useMemo(
    () => buffers.find((buffer) => buffer.id === activeBufferId) || null,
    [buffers, activeBufferId],
  );

  const explorerTargetPath = useMemo(
    () => getExplorerTargetPath(activeBuffer),
    [activeBuffer],
  );

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
