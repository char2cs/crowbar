import { useCallback } from "react";
import { useSettingsStore } from "@/features/settings/store";
import { useUIState } from "@/features/window/stores/ui-state-store";
import {
  resolveSidebarPaneTrigger,
  type SidebarTriggerSide,
  type SidebarView,
} from "@/features/layout/utils/sidebar-pane-utils";

interface OpenSidebarViewOptions {
  triggerSide?: SidebarTriggerSide;
}

export function useSidebarPaneController() {
  const isSidebarVisible = useUIState((s) => s.isSidebarVisible);
  const activeSidebarView = useUIState((s) => s.activeSidebarView);
  const setActiveView = useUIState((s) => s.setActiveView);
  const setIsSidebarVisible = useUIState((s) => s.setIsSidebarVisible);
  const settings = useSettingsStore((s) => s.settings);
  const updateSetting = useSettingsStore((s) => s.updateSetting);

  const openSidebarView = useCallback(
    (view: SidebarView, options: OpenSidebarViewOptions = {}) => {
      const { nextIsSidebarVisible, nextView, nextPosition } = resolveSidebarPaneTrigger(
        {
          isSidebarVisible,
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
      isSidebarVisible,
      setActiveView,
      setIsSidebarVisible,
      settings.sidebarPosition,
      updateSetting,
    ],
  );

  return { openSidebarView };
}
