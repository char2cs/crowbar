// web/src/routes/__root.tsx
import { createRootRoute, Outlet } from '@tanstack/react-router'
import { ErrorBoundary } from '@/components/error-boundary'
import { AnchoredToastProvider } from '@/components/ui/toast'
import { BrowserAgentPeer } from '@/features/browser/components/browser-agent-peer'
import { BrowserEventsListener } from '@/features/browser/components/browser-events-listener'
import { DaemonHaltGate } from '@/features/window/components/daemon-halt-gate'
import { DaemonHealthListener } from '@/features/window/components/daemon-health-listener'
import { ConnectionIndicator } from '@/components/layout/connection-indicator'
import { OperationsWatcher } from '@/components/layout/operations-watcher'

// No `toastManager` viewport here: SidebarToastOverlay (IDEShell or OOBE) is its only one, as a
// second renders every toast twice; the toast watchers live here so no route misses one. Boot
// hydration resolves before this mounts, so nothing renders against an empty pane layout.
function RootComponent() {
  return (
    <ErrorBoundary>
      <AnchoredToastProvider>
        <DaemonHealthListener />
        <ConnectionIndicator />
        <OperationsWatcher />
        <BrowserEventsListener />
        <BrowserAgentPeer />
        <DaemonHaltGate>
          <Outlet />
        </DaemonHaltGate>
      </AnchoredToastProvider>
    </ErrorBoundary>
  )
}

export const Route = createRootRoute({
  component: RootComponent,
})
