import { createFileRoute, redirect } from '@tanstack/react-router'
import { getMockWorkspace } from '@/lib/mock/workspaces'

export const Route = createFileRoute('/workspaces/$wsId/')({
  beforeLoad: ({ params }) => {
    const ws = getMockWorkspace(params.wsId)
    throw redirect({
      to: '/workspaces/$wsId/$step',
      params: { wsId: params.wsId, step: ws?.currentState ?? 'brainstorming' },
    })
  },
  component: () => null,
})
