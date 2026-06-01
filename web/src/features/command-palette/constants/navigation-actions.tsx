import {
  FileText,
  FolderOpen,
  Hash,
  Package,
} from "@phosphor-icons/react";
import type { SidebarView } from "@/features/layout/utils/sidebar-pane-utils";
import type { SettingsTab } from "@/features/window/stores/ui-state/types";
import type { Action } from "../models/action.types";

interface NavigationActionsParams {
  setIsSidebarVisible: (v: boolean) => void;
  setActiveView: (view: SidebarView) => void;
  setIsQuickOpenVisible: (v: boolean) => void;
  openSettingsDialog: (tab?: SettingsTab) => void;
  onClose: () => void;
}

export const createNavigationActions = (params: NavigationActionsParams): Action[] => {
  const {
    setIsSidebarVisible,
    setActiveView,
    setIsQuickOpenVisible,
    openSettingsDialog,
    onClose,
  } = params;

  return [
    {
      id: "view-show-files",
      label: "View: Show Files",
      description: "Switch to files view",
      icon: <FolderOpen />,
      category: "Navigation",
      commandId: "workbench.showFileExplorer",
      action: () => {
        setIsSidebarVisible(true);
        setActiveView("files");
        onClose();
      },
    },
    {
      id: "view-show-extensions",
      label: "View: Show Extensions",
      description: "Open extensions in settings",
      icon: <Package />,
      category: "Navigation",
      action: () => {
        onClose();
        openSettingsDialog("extensions");
      },
    },
    {
      id: "go-to-line",
      label: "Go: Go to Line",
      description: "Jump to a specific line number",
      icon: <Hash />,
      category: "Navigation",
      commandId: "editor.goToLine",
      action: () => {
        onClose();
        window.dispatchEvent(new CustomEvent("menu-go-to-line"));
      },
    },
    {
      id: "quick-open",
      label: "Go: Quick Open",
      description: "Jump to any file with fuzzy search",
      icon: <FileText />,
      category: "Navigation",
      commandId: "file.quickOpen",
      action: () => {
        onClose();
        setIsQuickOpenVisible(true);
      },
    },
  ];
};
