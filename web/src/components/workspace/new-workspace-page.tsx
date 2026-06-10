import { useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { toast } from 'sonner'
import { postWorkspace } from '@/lib/api'
import { WorkspaceCreationForm } from '@/components/workspace/WorkspaceCreationForm'
import { useSidebarStore } from '@/lib/store/sidebar'

const REPOS = [
  { id: 'crowbar', name: 'crowbar' },
  { id: 'quiver-core', name: 'quiver.core' },
  { id: 'quiver-desktop', name: 'quiver.desktop' },
]

export function NewWorkspacePage() {
  const navigate = useNavigate()
  const [loading, setLoading] = useState(false)
  const addWorkspace = useSidebarStore((s) => s.addWorkspace)

  const handleSubmit = async (data: { repoId: string; branch: string }) => {
    setLoading(true)
    try {
      const ws = await postWorkspace(data.repoId, data.branch)
      addWorkspace(data.repoId, ws.id, data.branch)
      void navigate({ to: '/workspaces/$wsId', params: { wsId: ws.id } })
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to create workspace')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="flex flex-1 items-center justify-center">
      <div className="w-full max-w-sm">
        <h1 className="mb-6 text-lg font-semibold text-foreground">New workspace</h1>
        <WorkspaceCreationForm repos={REPOS} onSubmit={handleSubmit} loading={loading} />
      </div>
    </div>
  )
}
