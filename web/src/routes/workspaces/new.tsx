import { useState } from 'react'
import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { flowsQueryOptions } from '@/lib/queries'
import { postWorkspace } from '@/lib/api'
import { WorkspaceCreationForm } from '@/components/workspace/WorkspaceCreationForm'

export const Route = createFileRoute('/workspaces/new')({
  component: NewWorkspacePage,
})

const REPOS = [
  { id: 'crowbar', name: 'crowbar' },
  { id: 'quiver-core', name: 'quiver.core' },
  { id: 'quiver-desktop', name: 'quiver.desktop' },
]

function NewWorkspacePage() {
  const navigate = useNavigate()
  const { data: flows = [] } = useQuery(flowsQueryOptions())
  const [loading, setLoading] = useState(false)

  const handleSubmit = async (data: { repoId: string; branch: string; flowName: string }) => {
    setLoading(true)
    const ws = await postWorkspace(data.repoId, data.branch, data.flowName)
    navigate({ to: '/workspaces/$wsId/$step', params: { wsId: ws.id, step: ws.currentState } })
  }

  return (
    <div className="flex flex-1 items-center justify-center">
      <div className="w-full max-w-sm">
        <h1 className="mb-6 text-lg font-semibold text-foreground">New workspace</h1>
        <WorkspaceCreationForm
          repos={REPOS}
          flows={flows.map(f => ({ name: f.name, description: f.description }))}
          onSubmit={handleSubmit}
          loading={loading}
        />
      </div>
    </div>
  )
}
