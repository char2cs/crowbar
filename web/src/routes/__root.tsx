// web/src/routes/__root.tsx
import { createRootRoute } from '@tanstack/react-router'
import { IDEShell } from '@/components/layout/IDEShell'
import { ErrorBoundary } from '@/components/ErrorBoundary'
import { HydrationGate } from '@/components/hydration-gate'
import { useProjectStore } from '@/lib/store/projects'

function RootComponent() {
  const activeProjectId = useProjectStore((s) => s.activeProjectId)
  return (
    <HydrationGate workspaceId={activeProjectId}>
      <ErrorBoundary>
        <IDEShell />
      </ErrorBoundary>
    </HydrationGate>
  )
}

export const Route = createRootRoute({
  component: RootComponent,
})
