import {
  FileText,
  FolderOpen,
  GlobeHemisphereWest as Globe,
  Plus,
  TerminalWindow as Terminal,
} from "@phosphor-icons/react";
import { useCallback } from "react";
import { createPortal } from "react-dom";
import { useBufferStore } from "@/features/editor/stores/buffer-store";
import { readFileContent } from "@/features/file-system/controllers/file-operations";
import { openFile } from "@/features/file-system/controllers/platform";
import { useFileSystemStore } from "@/features/file-system/controllers/store";
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
  const { openBuffer } = useBufferStore.use.actions();
  const bufferActions = useBufferActions();
  const handleOpenFolder = useFileSystemStore.use.handleOpenFolder();

  const contextMenu = useContextMenu();

  const handleOpenTerminal = useCallback(() => {
    bufferActions.openContent({ type: 'terminal' });
  }, [bufferActions]);

  const handleOpenWebViewer = useCallback(() => {
    bufferActions.openContent({ type: 'webViewer', url: 'https://' });
  }, [bufferActions]);

  const handleNewFile = useCallback(() => {
    const id = `untitled-${Date.now()}`;
    openBuffer(id, "Untitled", "", false, undefined, false, true);
    bufferActions.registerExternalBuffer(id, id, "Untitled", false);
  }, [bufferActions, openBuffer]);

  const handleOpenFile = useCallback(async () => {
    try {
      const selected = await openFile();
      if (selected && typeof selected === "string") {
        const fileName = selected.split("/").pop() || selected;
        const content = await readFileContent(selected);
        const bufferId = openBuffer(selected, fileName, content);
        bufferActions.registerExternalBuffer(bufferId, selected, fileName, false);
      }
    } catch (error) {
      console.error("Failed to open file:", error);
    }
  }, [bufferActions, openBuffer]);

  const getContextMenuItems = useCallback((): ContextMenuItem[] => {
    return [
      {
        id: "new-file",
        label: "New File",
        icon: <Plus />,
        onClick: handleNewFile,
      },
      {
        id: "open-folder",
        label: "Open Folder",
        icon: <FolderOpen />,
        onClick: handleOpenFolder,
      },
      {
        id: "open-file",
        label: "Open File",
        icon: <FileText />,
        onClick: handleOpenFile,
      },
      { id: "sep-1", label: "", separator: true, onClick: () => {} },
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
  }, [
    handleNewFile,
    handleOpenFolder,
    handleOpenFile,
    handleOpenTerminal,
    handleOpenWebViewer,
  ]);

  const actions: ActionItem[] = [
    {
      id: "new-file",
      label: "New File",
      icon: <Plus className="text-muted-foreground" />,
      action: handleNewFile,
    },
    {
      id: "folder",
      label: "Open Folder",
      icon: <FolderOpen className="text-muted-foreground" />,
      action: handleOpenFolder,
    },
    {
      id: "file",
      label: "Open File",
      icon: <FileText className="text-muted-foreground" />,
      action: handleOpenFile,
    },
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
