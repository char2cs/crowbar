import { createFileRoute, redirect } from '@tanstack/react-router'

export const Route = createFileRoute('/workspaces/$wsId/')({
  beforeLoad: ({ params }) => {
    throw redirect({ to: '/workspaces/$wsId', params: { wsId: params.wsId } })
  },
  component: () => null,
})
