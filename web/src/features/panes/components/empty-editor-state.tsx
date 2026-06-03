import {
  GlobeHemisphereWest as Globe,
  TerminalWindow as Terminal,
} from "@phosphor-icons/react";
import { useCallback } from "react";
import { createPortal } from "react-dom";
import { useBufferActions } from "@/features/workspace/stores/hooks/use-buffer-store";
import { Button } from "@/components/ui/button";
import { ContextMenu, useContextMenu, type ContextMenuItem } from "@/components/ui/context-menu";

interface ActionItem {
  id: string;
  label: string;
  icon: React.ReactNode;
  action: () => void;
}

const newTabRowClassName =
  "h-auto w-full justify-start gap-3 rounded-md px-3 py-1.5 text-left hover:bg-muted";

export function EmptyEditorState() {
  const bufferActions = useBufferActions();

  const contextMenu = useContextMenu();

  const handleOpenTerminal = useCallback(() => {
    bufferActions.openContent({ type: 'terminal' });
  }, [bufferActions]);

  const handleOpenWebViewer = useCallback(() => {
    bufferActions.openContent({ type: 'webViewer', url: 'https://' });
  }, [bufferActions]);

  const getContextMenuItems = useCallback((): ContextMenuItem[] => {
    return [
      {
        id: "new-terminal",
        label: "New Terminal",
        icon: <Terminal />,
        onClick: handleOpenTerminal,
      },
      {
        id: "open-url",
        label: "Open URL",
        icon: <Globe />,
        onClick: handleOpenWebViewer,
      },
    ];
  }, [handleOpenTerminal, handleOpenWebViewer]);

  const actions: ActionItem[] = [
    {
      id: "terminal",
      label: "New Terminal",
      icon: <Terminal className="text-muted-foreground" />,
      action: handleOpenTerminal,
    },
    {
      id: "web",
      label: "Open URL",
      icon: <Globe className="text-muted-foreground" />,
      action: handleOpenWebViewer,
    },
  ];

  return (
    <div className="flex h-full min-h-0 w-full overflow-auto" onContextMenu={contextMenu.open}>
      <div className="m-auto flex w-48 min-w-0 flex-col gap-0.5 px-2 py-3">
        {actions.map((item) => (
          <Button
            key={item.id}
            type="button"
            onClick={item.action}
            variant="ghost"
            className={newTabRowClassName}
            compact
          >
            <span className="shrink-0">{item.icon}</span>
            <span className="text-foreground ui-text-xs">{item.label}</span>
          </Button>
        ))}
      </div>

      {createPortal(
        <ContextMenu
          isOpen={contextMenu.isOpen}
          position={contextMenu.position}
          items={getContextMenuItems()}
          onClose={contextMenu.close}
        />,
        document.body,
      )}
    </div>
  );
}
