// web/src/routes/__root.tsx
import { createRootRoute, Outlet } from '@tanstack/react-router'
import { ErrorBoundary } from '@/components/error-boundary'
import { HydrationGate } from '@/components/hydration-gate'
import { AppSyncProvider } from '@/components/app-sync-provider'
import { AnchoredToastProvider } from '@/components/ui/toast'
import { DaemonHealthListener } from '@/features/window/components/daemon-health-listener'

// Note: there is deliberately NO global viewport for `toastManager` here.
// SidebarToastOverlay (inside IDEShell) is the one and only viewport for it —
// a second Toast.Provider on the same manager gives each provider its own copy
// of the toast list, so every toast renders twice whenever both are mounted.
// The anchored manager is separate and keeps its own root-level viewport.
function RootComponent() {
  return (
    <HydrationGate>
      <ErrorBoundary>
        <AppSyncProvider>
          <AnchoredToastProvider>
            <DaemonHealthListener />
            <Outlet />
          </AnchoredToastProvider>
        </AppSyncProvider>
      </ErrorBoundary>
    </HydrationGate>
  )
}

export const Route = createRootRoute({
  component: RootComponent,
})
