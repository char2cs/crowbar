// Tauri invoke replaced — CLI install is a no-op in web mode
const invoke = async (_cmd: string, _args?: unknown) => 'not available'
import {
  ArrowClockwise as RefreshCw,
  Sparkle as Sparkles,
  TerminalWindow as Terminal,
} from "@phosphor-icons/react";
import { useUIState } from "@/features/window/stores/ui-state-store";
import { primitiveAlert } from "@/components/ui/primitive-dialog-service";
import type { Action } from "../models/action.types";
import type { CommandPaletteViewId } from "../models/view.types";

interface AdvancedActionsParams {
  lspStatus: {
    status: string;
    activeWorkspaces: string[];
    lastError?: string | null | undefined;
  };
  updateLspStatus: (
    status: string,
    workspaces?: string[],
    error?: string,
    languages?: string[],
  ) => void;
  clearLspError: () => void;
  rootFolderPath: string | null | undefined;
  pushPaletteView: (view: CommandPaletteViewId) => void;
  showToast: (params: { message: string; type: "success" | "error" | "info" }) => void;
  onClose: () => void;
}

export const createAdvancedActions = (params: AdvancedActionsParams): Action[] => {
  const {
    lspStatus,
    updateLspStatus,
    clearLspError,
    rootFolderPath,
    pushPaletteView,
    showToast,
    onClose,
  } = params;

  const baseActions: Action[] = [
    {
      id: "ai-quick-question",
      label: "AI: Quick Question",
      description: "Ask a small question using the configured AI provider",
      icon: <Sparkles />,
      category: "AI",
      action: () => {
        pushPaletteView("quick-question");
      },
    },
    {
      id: "ai-new-agent",
      label: "AI: New Agent",
      description: "Open the unified agent launcher",
      icon: <Sparkles />,
      category: "AI",
      commandId: "workbench.agentLauncher",
      action: () => {
        useUIState.getState().setIsAgentLauncherVisible(true);
        onClose();
      },
    },
    {
      id: "lsp-status",
      label: "LSP: Show Status",
      description: `Status: ${lspStatus.status} (${lspStatus.activeWorkspaces.length} workspaces)`,
      icon: <Terminal />,
      category: "LSP",
      action: async () => {
        await primitiveAlert(
          `LSP Status: ${lspStatus.status}\nActive workspaces: ${lspStatus.activeWorkspaces.join(", ") || "None"}\nError: ${lspStatus.lastError || "None"}`,
          "LSP Status",
        );
        onClose();
      },
    },
    {
      id: "lsp-restart",
      label: "LSP: Restart Server",
      description: "Restart the LSP server",
      icon: <RefreshCw />,
      category: "LSP",
      action: () => {
        updateLspStatus("connecting");
        clearLspError();
        setTimeout(() => {
          updateLspStatus("connected", [rootFolderPath || ""]);
        }, 1000);
        onClose();
      },
    },
    {
      id: "cli-install",
      label: "CLI: Install Terminal Command",
      description: "Install 'athas' command for terminal",
      icon: <Terminal />,
      category: "CLI",
      action: async () => {
        try {
          showToast({ message: "Installing CLI command...", type: "info" });
          const result = await invoke("install_cli_command") as string;
          showToast({ message: result, type: "success" });
        } catch (error) {
          showToast({
            message: `Failed to install CLI: ${error}. You may need administrator privileges.`,
            type: "error",
          });
        }
        onClose();
      },
    },
  ];

  return baseActions;
};
