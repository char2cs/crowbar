import { createFileRoute } from '@tanstack/react-router'

export const Route = createFileRoute('/workspaces/$wsId')({
  component: () => null, // IDEShell handles rendering via WorkspaceView
})
