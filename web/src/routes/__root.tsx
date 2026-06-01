// web/src/routes/__root.tsx
import { createRootRoute } from '@tanstack/react-router'
import { IDEShell } from '@/components/layout/IDEShell'
import { ErrorBoundary } from '@/components/ErrorBoundary'
import { HydrationGate } from '@/components/hydration-gate'

function RootComponent() {
  return (
    <HydrationGate>
      <ErrorBoundary>
        <IDEShell />
      </ErrorBoundary>
    </HydrationGate>
  )
}

export const Route = createRootRoute({
  component: RootComponent,
})
