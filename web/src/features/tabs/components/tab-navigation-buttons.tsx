import {
  ArrowLeft,
  ArrowRight,
  SidebarSimple as PanelLeftClose,
} from "@phosphor-icons/react";
import React from "react";
import { Button } from "@/components/ui/button";
import { cn } from "@/utils/cn";

interface TabNavigationButtonsProps {
  /** Whether the tab bar is in the bottom pane (hides sidebar toggle) */
  isBottomPane: boolean;
  /** Whether the sidebar is currently open */
  sidebarOpen: boolean;
  /** Which side the sidebar is on */
  sidebarPosition: "left" | "right";
  /** Whether the tab bar is flush with the left edge of the window */
  isAtLeftEdge: boolean;
  canGoBack: boolean;
  canGoForward: boolean;
  onToggleSidebar: () => void;
  onJumpBack: () => void;
  onJumpForward: () => void;
}

const TabNavigationButtons = React.memo(function TabNavigationButtons({
  isBottomPane,
  sidebarOpen,
  sidebarPosition,
  canGoBack,
  canGoForward,
  onToggleSidebar,
  onJumpBack,
  onJumpForward,
}: TabNavigationButtonsProps) {
  return (
    <>
      {!isBottomPane && (
        <Button
          type="button"
          onClick={onToggleSidebar}
          variant="ghost"
          compact
          className={cn(
            "h-6 w-6 shrink-0 rounded-full p-0 text-muted-foreground",
            sidebarPosition === "right" && "scale-x-[-1]",
          )}
          tooltip={sidebarOpen ? "Hide Sidebar" : "Show Sidebar"}
          tooltipSide="bottom"
          aria-label={sidebarOpen ? "Hide sidebar" : "Show sidebar"}
        >
          <PanelLeftClose size={14} />
        </Button>
      )}

      <div className="flex shrink-0 items-center gap-0.5">
        <Button
          type="button"
          onClick={onJumpBack}
          disabled={!canGoBack}
          variant="ghost"
          className="h-6 w-6 shrink-0 rounded-full p-0 text-muted-foreground"
          tooltip="Go Back"
          tooltipSide="bottom"
          commandId="navigation.goBack"
          aria-label="Go back to previous location"
          compact
        >
          <ArrowLeft />
        </Button>
        <Button
          type="button"
          onClick={onJumpForward}
          disabled={!canGoForward}
          variant="ghost"
          className="h-6 w-6 shrink-0 rounded-full p-0 text-muted-foreground"
          tooltip="Go Forward"
          tooltipSide="bottom"
          commandId="navigation.goForward"
          aria-label="Go forward to next location"
          compact
        >
          <ArrowRight />
        </Button>
      </div>
    </>
  );
});

export default TabNavigationButtons;
