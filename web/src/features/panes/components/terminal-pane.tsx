import { lazy, Suspense } from "react";

const TerminalTab = lazy(() =>
  import("@/features/terminal/components/terminal-tab").then((m) => ({
    default: m.TerminalTab,
  })),
);

interface TerminalPaneProps {
  sessionId: string | undefined;
  bufferId: string;
  paneId: string;
  initialCommand?: string;
  workingDirectory?: string;
  remoteConnectionId?: string;
  isActive: boolean;
  isVisible?: boolean;
}

export function TerminalPane({
  sessionId,
  bufferId,
  paneId,
  initialCommand,
  workingDirectory,
  remoteConnectionId,
  isActive,
  isVisible,
}: TerminalPaneProps) {
  if (!sessionId) return null;
  return (
    <Suspense fallback={null}>
      <TerminalTab
        sessionId={sessionId}
        bufferId={bufferId}
        paneId={paneId}
        initialCommand={initialCommand}
        workingDirectory={workingDirectory}
        remoteConnectionId={remoteConnectionId}
        isActive={isActive}
        isVisible={isVisible}
      />
    </Suspense>
  );
}
