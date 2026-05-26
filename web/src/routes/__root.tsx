// web/src/routes/__root.tsx
import { createRootRoute } from '@tanstack/react-router'
import { IDEShell } from '@/components/layout/IDEShell'
import { ErrorBoundary } from '@/components/ErrorBoundary'

export const Route = createRootRoute({
  component: () => (
    <ErrorBoundary>
      <IDEShell />
    </ErrorBoundary>
  ),
})
