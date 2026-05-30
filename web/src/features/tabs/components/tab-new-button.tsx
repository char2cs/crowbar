import {
  GlobeHemisphereWest as Globe,
  Plus,
  SidebarSimple as PanelLeftClose,
  TerminalWindow as Terminal,
} from "@phosphor-icons/react";
import React from "react";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Button } from "@/components/ui/button";

interface TabNewButtonProps {
  /** The pane this button belongs to — required for actions */
  paneId: string;
  /** Whether the pane is the bottom pane (hides this button) */
  isBottomPane: boolean;
  /** Whether pane actions (close split) are disabled */
  disablePaneActions: boolean;
  /** Whether the workspace is currently split */
  isInSplit: boolean;
  onNewTerminal: () => void;
  onOpenUrl: () => void;
  onClosePane: () => void;
}

const TabNewButton = React.memo(function TabNewButton({
  isBottomPane,
  disablePaneActions,
  isInSplit,
  onNewTerminal,
  onOpenUrl,
  onClosePane,
}: TabNewButtonProps) {
  if (isBottomPane) return null;

  return (
    <div className="flex shrink-0 items-center gap-1 pl-0.5">
      <DropdownMenu>
        <DropdownMenuTrigger
          className="flex h-6 w-6 shrink-0 items-center justify-center rounded-full p-0 text-muted-foreground hover:bg-muted hover:text-foreground focus:outline-none"
          aria-label="New tab"
        >
          <Plus weight="bold" size={12} />
        </DropdownMenuTrigger>
        <DropdownMenuContent side="bottom" align="start" className="min-w-[140px]">
          <DropdownMenuItem onClick={onNewTerminal}>
            <Terminal className="text-muted-foreground" />
            New Terminal
          </DropdownMenuItem>
          <DropdownMenuItem onClick={onOpenUrl}>
            <Globe className="text-muted-foreground" />
            Open URL
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>

      {!disablePaneActions && isInSplit && (
        <Button
          type="button"
          onClick={onClosePane}
          variant="ghost"
          compact
          className="h-6 w-6 shrink-0 rounded-full p-0 text-muted-foreground"
          tooltip="Close Split"
          tooltipSide="bottom"
          aria-label="Close split pane"
        >
          <PanelLeftClose />
        </Button>
      )}
    </div>
  );
});

export default TabNewButton;
