// web/src/routes/__root.tsx
import { createRootRoute, Outlet } from '@tanstack/react-router'
import { ErrorBoundary } from '@/components/error-boundary'
import { HydrationGate } from '@/components/hydration-gate'
import { AppSyncProvider } from '@/components/app-sync-provider'
import { Toaster } from '@/components/ui/sonner'

function RootComponent() {
  return (
    <HydrationGate>
      <ErrorBoundary>
        <AppSyncProvider>
          <Outlet />
          {/* Mounted at the root so toasts render on every route (OOBE,
              onboarding, the IDE shell, etc.) — not just inside the shell. */}
          <Toaster />
        </AppSyncProvider>
      </ErrorBoundary>
    </HydrationGate>
  )
}

export const Route = createRootRoute({
  component: RootComponent,
})
