import {
  ArrowLeft,
  ArrowRight,
  SidebarSimple as PanelLeftClose,
} from "@phosphor-icons/react";
import React from "react";
import { Button } from "@/components/ui/button";
import { cn } from "@/utils/cn";

interface TabNavigationButtonsProps {
  isBottomPane: boolean;
  sidebarOpen: boolean;
  sidebarPosition: "left" | "right";
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
          onClick={onToggleSidebar}
          variant="ghost"
          size="icon-xs"
          className={cn(
            "shrink-0 text-muted-foreground",
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
          onClick={onJumpBack}
          disabled={!canGoBack}
          variant="ghost"
          size="icon-xs"
          className="shrink-0 text-muted-foreground"
          tooltip="Go Back"
          tooltipSide="bottom"
          aria-label="Go back to previous location"
        >
          <ArrowLeft />
        </Button>
        <Button
          onClick={onJumpForward}
          disabled={!canGoForward}
          variant="ghost"
          size="icon-xs"
          className="shrink-0 text-muted-foreground"
          tooltip="Go Forward"
          tooltipSide="bottom"
          aria-label="Go forward to next location"
        >
          <ArrowRight />
        </Button>
      </div>
    </>
  );
});

export default TabNavigationButtons;
