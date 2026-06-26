// web/src/routes/__root.tsx
import { createRootRoute, Outlet } from '@tanstack/react-router'
import { ErrorBoundary } from '@/components/error-boundary'
import { HydrationGate } from '@/components/hydration-gate'
import { AppSyncProvider } from '@/components/app-sync-provider'
import { AnchoredToastProvider, ToastProvider } from '@/components/ui/toast'

function RootComponent() {
  return (
    <HydrationGate>
      <ErrorBoundary>
        <AppSyncProvider>
          {/* ToastProvider + AnchoredToastProvider mounted at the root so
              toasts render on every route (OOBE, onboarding, IDE shell, etc.) */}
          <ToastProvider position="bottom-right">
            <AnchoredToastProvider>
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
