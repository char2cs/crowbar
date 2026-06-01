import { useEffect } from "react";
import { IS_MAC } from "@/utils/platform";
import { splitActiveEditorGroup } from "../utils/pane-command-actions";
import { useWorkspaceStore } from "@/features/workspace/stores/workspace-context";

export function usePaneKeyboard() {
  const workspaceStore = useWorkspaceStore();

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      const modKey = IS_MAC ? e.metaKey : e.ctrlKey;

      if (!modKey) return;

      // Cmd+\ or Ctrl+\ - Split right
      if (e.key === "\\" && !e.shiftKey) {
        e.preventDefault();
        splitActiveEditorGroup("horizontal");
        return;
      }

      // Cmd+Shift+\ or Ctrl+Shift+\ - Split down
      if (e.key === "\\" && e.shiftKey) {
        e.preventDefault();
        splitActiveEditorGroup("vertical");
        return;
      }

      // Cmd+Option+Arrow or Ctrl+Alt+Arrow - Navigate between panes
      if (e.altKey && ["ArrowLeft", "ArrowRight", "ArrowUp", "ArrowDown"].includes(e.key)) {
        e.preventDefault();
        const directionMap: Record<string, "left" | "right" | "up" | "down"> = {
          ArrowLeft: "left",
          ArrowRight: "right",
          ArrowUp: "up",
          ArrowDown: "down",
        };
        workspaceStore.getState().paneActions.navigateToPane(directionMap[e.key]);
        return;
      }
    };

    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [workspaceStore]);
}
