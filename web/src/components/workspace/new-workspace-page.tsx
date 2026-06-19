import { useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { toast } from 'sonner'
import { postWorkspace } from '@/lib/api'
import { awaitEntity } from '@/lib/ws/await-entity'
import { WorkspaceCreationForm } from '@/components/workspace/workspace-creation-form'
import { useSidebarStore } from '@/lib/store/sidebar'
import type { WorkspaceDTO } from '@/lib/types'

export function NewWorkspacePage() {
  const [loading, setLoading] = useState(false)
  const navigate = useNavigate()
  // §7: source the repo list from the WS-driven sidebar cache rather than a
  // hardcoded mock — only repos the user actually has can host a new workspace.
  const repos = useSidebarStore((s) => s.repos)
  const formRepos = repos.map((r) => ({ id: r.id, name: r.name }))

  const handleSubmit = async (data: { repoId: string; branch: string }) => {
    setLoading(true)
    try {
      const repo = useSidebarStore.getState().repos.find((r) => r.id === data.repoId)
      const projectId = repo?.projectId ?? ''
      // §3/§4 subscribe-before-POST: postWorkspace answers 202; the ready
      // WorkspaceDTO arrives on the per-repo workspaces stream. Subscribe first,
      // fire the POST, then navigate once the new branch's DTO lands.
      const ws = await awaitEntity<WorkspaceDTO>({
        endpoint: `/v0/projects/${projectId}/repos/${data.repoId}/workspaces`,
        match: (w) => w.branch === data.branch && w.status !== 'deleted',
        action: () => postWorkspace(projectId, data.repoId, data.branch),
      })
      void navigate({
        to: '/ide/$projectId/$repoId/$wsId',
        params: { projectId, repoId: data.repoId, wsId: ws.id },
      })
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
        <WorkspaceCreationForm repos={formRepos} onSubmit={handleSubmit} loading={loading} />
      </div>
    </div>
  )
}
