import { useCallback } from "react";
import { useSettingsStore } from "@/features/settings/store";
import { useUIState } from "@/features/window/stores/ui-state-store";
import {
  getSidebarPaneLevel,
  resolveSidebarPaneTrigger,
  type SidebarPaneLevel,
  type SidebarTriggerSide,
  type SidebarView,
} from "@/features/layout/utils/sidebar-pane-utils";

interface OpenSidebarViewOptions {
  paneLevel?: SidebarPaneLevel;
  triggerSide?: SidebarTriggerSide;
}

export function useSidebarPaneController() {
  const isSidebarVisible = useUIState((s) => s.isSidebarVisible);
  const isRightSidebarVisible = useUIState((s) => s.isRightSidebarVisible);
  const isGitViewActive = useUIState((s) => s.isGitViewActive);
  const isGitHubPRsViewActive = useUIState((s) => s.isGitHubPRsViewActive);
  const activeSidebarView = useUIState((s) => s.activeSidebarView);
  const activeRightSidebarView = useUIState((s) => s.activeRightSidebarView);
  const setActiveView = useUIState((s) => s.setActiveView);
  const setActiveRightSidebarView = useUIState((s) => s.setActiveRightSidebarView);
  const setIsSidebarVisible = useUIState((s) => s.setIsSidebarVisible);
  const setIsRightSidebarVisible = useUIState((s) => s.setIsRightSidebarVisible);
  const settings = useSettingsStore((s) => s.settings);
  const updateSetting = useSettingsStore((s) => s.updateSetting);

  const openSidebarView = useCallback(
    (view: SidebarView, options: OpenSidebarViewOptions = {}) => {
      const paneLevel = options.paneLevel ?? getSidebarPaneLevel(view);

      if (paneLevel === "edge") {
        setActiveRightSidebarView(view);
        setIsRightSidebarVisible(!(isRightSidebarVisible && activeRightSidebarView === view));
        return;
      }

      const { nextIsSidebarVisible, nextView, nextPosition } = resolveSidebarPaneTrigger(
        {
          isSidebarVisible,
          isGitViewActive,
          isGitHubPRsViewActive,
          activeSidebarView: activeSidebarView ?? undefined,
        },
        view,
        {
          currentPosition: settings.sidebarPosition,
          triggerSide: options.triggerSide,
        },
      );

      if (settings.sidebarPosition !== nextPosition) {
        void updateSetting("sidebarPosition", nextPosition);
      }

      setActiveView(nextView);
      setIsSidebarVisible(nextIsSidebarVisible);
    },
    [
      activeSidebarView,
      activeRightSidebarView,
      isGitHubPRsViewActive,
      isGitViewActive,
      isRightSidebarVisible,
      isSidebarVisible,
      setActiveView,
      setActiveRightSidebarView,
      setIsSidebarVisible,
      setIsRightSidebarVisible,
      settings.sidebarPosition,
      updateSetting,
    ],
  );

  return { openSidebarView };
}
