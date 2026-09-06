import { lazy, Suspense } from 'react'

const TerminalTab = lazy(() =>
  import('@/features/terminal/components/terminal-tab').then((m) => ({
    default: m.TerminalTab,
  })),
)

interface TerminalPaneProps {
  sessionId: string | undefined
  bufferId: string
  paneId: string
  /** The workspace THIS BUFFER belongs to (TerminalContent.workspaceId) —
   *  threaded straight through to TerminalTab so it never has to fall back to
   *  ambient context. See terminal-tab.tsx's own doc comment for why. */
  workspaceId: string
  initialCommand?: string
  workingDirectory?: string
  remoteConnectionId?: string
  isActive: boolean
  isVisible?: boolean
}

export function TerminalPane({
  sessionId,
  bufferId,
  paneId,
  workspaceId,
  initialCommand,
  workingDirectory,
  remoteConnectionId,
  isActive,
  isVisible,
}: TerminalPaneProps) {
  if (!sessionId) return null
  return (
    <Suspense fallback={null}>
      <TerminalTab
        sessionId={sessionId}
        bufferId={bufferId}
        paneId={paneId}
        workspaceId={workspaceId}
        initialCommand={initialCommand}
        workingDirectory={workingDirectory}
        remoteConnectionId={remoteConnectionId}
        isActive={isActive}
        isVisible={isVisible}
      />
    </Suspense>
  )
}
