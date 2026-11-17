import { createRootRoute, Outlet } from '@tanstack/react-router'
import { ErrorBoundary } from '@/components/error-boundary'
import { AnchoredToastProvider } from '@/components/ui/toast'
import { DaemonHaltGate } from '@/features/window/components/daemon-halt-gate'
import { ConnectionIndicator } from '@/components/layout/connection-indicator'
import { OperationsWatcher } from '@/components/layout/operations-watcher'

// No `toastManager` viewport here: SidebarToastOverlay (in the phone shell) is its only one, as a
// second renders every toast twice; the toast watchers live here so no route misses one. Boot
// hydration resolves before this mounts, so nothing renders against an empty pane layout.
function RootComponent() {
  return (
    <ErrorBoundary>
      <AnchoredToastProvider>
        <ConnectionIndicator />
        <OperationsWatcher />
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
