import { useCallback, useEffect, useRef } from "react";
import { useWorkspaceStore } from "@/features/workspace/stores/workspace-context";
import { useUIState } from "@/features/window/stores/ui-state-store";
import { XtermTerminal } from "./terminal";

interface TerminalTabProps {
  sessionId: string;
  bufferId: string;
  paneId?: string;
  initialCommand?: string;
  workingDirectory?: string;
  remoteConnectionId?: string;
  isActive?: boolean;
  isVisible?: boolean;
}

// Renders XtermTerminal directly — no portal indirection. The TerminalHost
// portal mechanism (TerminalSlot → XtermPortal) is reserved for the bottom
// panel terminal system where PTY sessions must survive pane rearrangements.
// Workspace pane terminals don't have that constraint yet.
export function TerminalTab({
  sessionId,
  bufferId,
  paneId,
  initialCommand,
  workingDirectory,
  remoteConnectionId,
  isActive = true,
  isVisible = true,
}: TerminalTabProps) {
  const workspaceStore = useWorkspaceStore();
  const registerTerminalFocus = useUIState(s => s.registerTerminalFocus);
  const clearTerminalFocus = useUIState(s => s.clearTerminalFocus);
  const focusFnRef = useRef<(() => void) | null>(null);

  const handleTerminalExit = useCallback(() => {
    workspaceStore.getState().bufferActions.closeBuffer(bufferId);
  }, [bufferId, workspaceStore]);

  const handleActivate = useCallback(() => {
    if (paneId) {
      workspaceStore.getState().paneActions.addBufferToPane(paneId, bufferId, true);
      return;
    }
    workspaceStore.getState().paneActions.activatePaneBuffer(
      workspaceStore.getState().activePaneId,
      bufferId,
    );
  }, [bufferId, paneId, workspaceStore]);

  const handleTerminalRef = useCallback(
    (ref: { focus: () => void; showSearch: () => void; terminal: unknown } | null) => {
      if (ref) {
        focusFnRef.current = ref.focus;
      } else {
        focusFnRef.current = null;
      }
    },
    [],
  );

  // Register this terminal's focus function with the global registry so
  // requestTerminalFocus() can target it when "Show Terminal" is triggered.
  useEffect(() => {
    registerTerminalFocus(bufferId, () => {
      focusFnRef.current?.();
    });
    return () => clearTerminalFocus(bufferId);
  }, [bufferId, registerTerminalFocus, clearTerminalFocus]);

  return (
    // onMouseDownCapture: xterm canvas events don't bubble through React, so we
    // capture the native mousedown here to activate the pane without fighting
    // the portal boundary.
    <div
      className="flex h-full w-full flex-col overflow-hidden"
      onMouseDownCapture={handleActivate}
    >
      <XtermTerminal
        sessionId={sessionId}
        isActive={isActive}
        isVisible={isVisible}
        onTerminalExit={handleTerminalExit}
        onTerminalRef={handleTerminalRef}
        initialCommand={initialCommand}
        workingDirectory={workingDirectory}
        remoteConnectionId={remoteConnectionId}
      />
    </div>
  );
}
