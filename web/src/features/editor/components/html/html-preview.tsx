import { useEffect, useRef, useState } from "react";
import { useShallow } from "zustand/react/shallow";
import { useWorkspaceStoreContext } from "@/features/workspace/stores/workspace-context";
import { findPaneGroup } from "@/features/panes/utils/pane-tree";
import { useFileSystemStore } from "@/features/file-system/controllers/store";
import { hasTextContent } from "@/features/panes/types/pane-content";
import { buildHtmlPreviewDocument } from "./html-preview-document";

export function HtmlPreview() {
  const { hasSourceBuffer, sourceContent, sourcePath } = useWorkspaceStoreContext(
    useShallow((state) => {
      const activeBufferId = findPaneGroup(state.paneRoot, state.activePaneId)?.activeBufferId ?? null;
      const activeBuffer = activeBufferId
        ? state.buffers.find((buffer) => buffer.id === activeBufferId)
        : null;
      const sourceBuffer =
        activeBuffer?.type === "htmlPreview"
          ? (state.buffers.find((buffer) => buffer.path === activeBuffer.sourceFilePath) ??
            activeBuffer)
          : activeBuffer;

      return {
        hasSourceBuffer: Boolean(sourceBuffer),
        sourceContent: sourceBuffer && hasTextContent(sourceBuffer) ? sourceBuffer.content : "",
        sourcePath: sourceBuffer?.path,
      };
    }),
  );
  const rootFolderPath = useFileSystemStore.use.rootFolderPath?.();

  const [iframeContent, setIframeContent] = useState("");
  const containerRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    setIframeContent(buildHtmlPreviewDocument(sourceContent, { sourcePath, rootFolderPath: rootFolderPath ?? undefined }));
  }, [sourceContent, sourcePath, rootFolderPath]);

  if (!hasSourceBuffer) {
    return (
      <div className="flex h-full items-center justify-center text-muted-foreground">
        No active buffer
      </div>
    );
  }

  return (
    <div ref={containerRef} className="html-preview h-full w-full bg-white">
      <iframe
        title="HTML Preview"
        srcDoc={iframeContent}
        className="h-full w-full border-none"
        sandbox="allow-scripts allow-same-origin allow-forms allow-popups allow-modals"
      />
    </div>
  );
}
