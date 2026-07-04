// web/src/routes/__root.tsx
import { createRootRoute, Outlet } from '@tanstack/react-router'
import { ErrorBoundary } from '@/components/error-boundary'
import { HydrationGate } from '@/components/hydration-gate'
import { AppSyncProvider } from '@/components/app-sync-provider'
import { AnchoredToastProvider, ToastProvider } from '@/components/ui/toast'
import { DaemonHealthListener } from '@/features/window/components/daemon-health-listener'
import { useUIState } from '@/features/window/stores/ui-state-store'

function RootComponent() {
  const ideShellMounted = useUIState((s) => s.ideShellMounted)
  return (
    <HydrationGate>
      <ErrorBoundary>
        <AppSyncProvider>
          <ToastProvider position="bottom-right" suppressToasts={ideShellMounted}>
            <AnchoredToastProvider>
              <DaemonHealthListener />
              <Outlet />
            </AnchoredToastProvider>
          </ToastProvider>
        </AppSyncProvider>
      </ErrorBoundary>
    </HydrationGate>
  )
}

export const Route = createRootRoute({
  component: RootComponent,
})
