import { createFileRoute } from '@tanstack/react-router'
import { NewWorkspacePage } from '@/components/workspace/new-workspace-page'

export const Route = createFileRoute('/workspaces/new')({
  component: NewWorkspacePage,
})
