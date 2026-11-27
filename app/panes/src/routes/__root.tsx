import { createRootRoute, Outlet } from '@tanstack/react-router'
import { ErrorBoundary } from '@/components/error-boundary'
import { AnchoredToastProvider } from '@/components/ui/toast'
import { DaemonHaltGate } from '@/features/window/components/daemon-halt-gate'
import { ConnectionIndicator } from '@/components/layout/connection-indicator'
import { OperationsWatcher } from '@/components/layout/operations-watcher'

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
