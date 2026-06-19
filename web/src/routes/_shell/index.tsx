import { useState } from 'react'
import { GitBranchIcon } from 'lucide-react'
import { createFileRoute, redirect } from '@tanstack/react-router'
import { fetchProjects, fetchRepos, fetchWorkspaces } from '@/lib/api'
import { useProjectStore } from '@/lib/store/projects'
import {
  Empty,
  EmptyMedia,
  EmptyHeader,
  EmptyTitle,
  EmptyDescription,
} from '@/components/ui/empty'
import { Button } from '@/components/ui/button'
import { AddRepositoryModal } from '@/components/projects/add-repository-modal'

function NoReposScreen() {
  const [open, setOpen] = useState(false)

  return (
    <>
      <Empty>
        <EmptyMedia variant="icon">
          <GitBranchIcon />
        </EmptyMedia>
        <EmptyHeader>
          <EmptyTitle>No repositories yet</EmptyTitle>
          <EmptyDescription>Add a git repository to open a workspace.</EmptyDescription>
        </EmptyHeader>
        <Button onClick={() => setOpen(true)}>Add repository</Button>
      </Empty>
      <AddRepositoryModal open={open} onOpenChange={setOpen} />
    </>
  )
}

export const Route = createFileRoute('/_shell/')({
  component: NoReposScreen,
  beforeLoad: async () => {
    // §7 landing: there is no flat cross-project workspace list anymore. Resolve
    // the landing route from the per-project repos/workspaces hierarchy, biasing
    // toward the persisted active project so a returning user lands where they
    // left off.
    const projects = await fetchProjects()
    if (projects.length === 0) {
      throw redirect({ to: '/oobe' })
    }

    const activeId = useProjectStore.getState().activeProjectId
    const project = projects.find((p) => p.id === activeId) ?? projects[0]

    // Walk this project's repos for the first workspace we can land on, biasing
    // toward an editable (non-locked) workspace so editing works out of the box.
    try {
      const repos = await fetchRepos(project.id)
      for (const repo of repos) {
        const workspaces = await fetchWorkspaces(project.id, repo.id)
        if (workspaces.length === 0) continue
        const editable = workspaces.find((ws) => ws.status !== 'locked')
        const ws = editable ?? workspaces[0]
        throw redirect({
          to: '/ide/$projectId/$repoId/$wsId',
          params: { projectId: project.id, repoId: repo.id, wsId: ws.id },
        })
      }
    } catch (err) {
      // A thrown redirect is the success path — re-throw it. Any other failure
      // (a transient repos/workspaces fetch error) falls through to the
      // NoReposScreen rather than crashing cold start.
      if (err && typeof err === 'object' && 'to' in err) throw err
    }
    // Has projects but no landable workspace — render NoReposScreen.
  },
})
