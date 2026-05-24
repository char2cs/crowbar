// web/src/routes/workspaces/$wsId.tsx
import { createFileRoute, Outlet } from '@tanstack/react-router'

export const Route = createFileRoute('/workspaces/$wsId')({
  component: () => <Outlet />,
})
