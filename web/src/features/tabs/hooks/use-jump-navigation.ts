import { useCallback } from "react";
import { useJumpListStore } from "@/features/editor/stores/jump-list-store";
import { useEditorStateStore } from "@/features/editor/stores/state-store";
import { navigateToJumpEntry } from "@/features/editor/utils/jump-navigation";
import { useWorkspaceStore } from "@/features/workspace/stores/workspace-context";

interface WebViewerNavigation {
  canGoBack?: boolean;
  canGoForward?: boolean;
  goBack?: () => void;
  goForward?: () => void;
}

interface UseJumpNavigationOptions {
  usesWebViewerNavigation: boolean;
  activeWebViewerNavigation: WebViewerNavigation | undefined;
}

/**
 * Returns stable `handleJumpBack` and `handleJumpForward` callbacks that
 * delegate to the web-viewer navigation or the editor jump-list.
 */
export function useJumpNavigation({
  usesWebViewerNavigation,
  activeWebViewerNavigation,
}: UseJumpNavigationOptions) {
  const jumpListActions = useJumpListStore.use.actions();
  const workspaceStore = useWorkspaceStore();

  const canGoBack = usesWebViewerNavigation
    ? Boolean(activeWebViewerNavigation?.canGoBack)
    : jumpListActions.canGoBack();

  const canGoForward = usesWebViewerNavigation
    ? Boolean(activeWebViewerNavigation?.canGoForward)
    : jumpListActions.canGoForward();

  const handleJumpBack = useCallback(async () => {
    if (usesWebViewerNavigation) {
      activeWebViewerNavigation?.goBack?.();
      return;
    }

    const wsState = workspaceStore.getState();
    const editorState = useEditorStateStore.getState();
    const currentActiveBufferId = wsState.panes[wsState.activePaneId]?.activeBufferId ?? null;
    const currentActiveBuffer = currentActiveBufferId
      ? wsState.buffers.find((b) => b.id === currentActiveBufferId)
      : undefined;

    const currentPosition =
      currentActiveBufferId && currentActiveBuffer?.path
        ? {
            bufferId: currentActiveBufferId,
            filePath: currentActiveBuffer.path,
            line: editorState.cursorPosition.line,
            column: editorState.cursorPosition.column,
            offset: editorState.cursorPosition.offset,
            scrollTop: editorState.scrollTop,
            scrollLeft: editorState.scrollLeft,
          }
        : undefined;

    const entry = jumpListActions.goBack(currentPosition);
    if (entry) {
      await navigateToJumpEntry(entry);
    }
  }, [activeWebViewerNavigation, jumpListActions, usesWebViewerNavigation, workspaceStore]);

  const handleJumpForward = useCallback(async () => {
    if (usesWebViewerNavigation) {
      activeWebViewerNavigation?.goForward?.();
      return;
    }

    const entry = jumpListActions.goForward();
    if (entry) {
      await navigateToJumpEntry(entry);
    }
  }, [activeWebViewerNavigation, jumpListActions, usesWebViewerNavigation]);

  return { canGoBack, canGoForward, handleJumpBack, handleJumpForward };
}
