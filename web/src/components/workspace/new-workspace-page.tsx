import { useState } from 'react'
import { toast } from 'sonner'
import { postWorkspace } from '@/lib/api'
import { WorkspaceCreationForm } from '@/components/workspace/workspace-creation-form'
import { useSidebarStore } from '@/lib/store/sidebar'

const REPOS = [
  { id: 'crowbar', name: 'crowbar' },
  { id: 'quiver-core', name: 'quiver.core' },
  { id: 'quiver-desktop', name: 'quiver.desktop' },
]

export function NewWorkspacePage() {
  const [loading, setLoading] = useState(false)

  const handleSubmit = async (data: { repoId: string; branch: string }) => {
    setLoading(true)
    try {
      // §3: postWorkspace is 202 — the WorkspaceDTO arrives over the §7 stream
      // and the WS-driven cache inserts it. Threading projectId from the cached
      // repo; navigation-after-WS-DTO and cache-sourced repos are W18.
      const projectId =
        useSidebarStore.getState().repos.find((r) => r.id === data.repoId)?.projectId ?? ''
      await postWorkspace(projectId, data.repoId, data.branch)
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
