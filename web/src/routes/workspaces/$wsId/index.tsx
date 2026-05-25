import { createFileRoute } from '@tanstack/react-router'

// Step is now Zustand state, not part of the URL.
// This index route no longer needs to redirect anywhere — IDEShell renders
// WorkspaceView directly based on the wsId path segment.
export const Route = createFileRoute('/workspaces/$wsId/')({
  component: () => null,
})
