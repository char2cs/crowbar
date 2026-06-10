// web/src/routes/__root.tsx
import { createRootRoute } from '@tanstack/react-router'
import { IDEShell } from '@/components/layout/ide-shell'
import { ErrorBoundary } from '@/components/error-boundary'
import { HydrationGate } from '@/components/hydration-gate'
import { AppSyncProvider } from '@/components/app-sync-provider'

function RootComponent() {
  return (
    <HydrationGate>
      <ErrorBoundary>
        <AppSyncProvider>
          <IDEShell />
        </AppSyncProvider>
      </ErrorBoundary>
    </HydrationGate>
  )
}

export const Route = createRootRoute({
  component: RootComponent,
})
