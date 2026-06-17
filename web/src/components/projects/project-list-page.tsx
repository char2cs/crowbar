import { useState } from 'react'
import { GitBranchIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  Empty,
  EmptyMedia,
  EmptyHeader,
  EmptyTitle,
  EmptyDescription,
  EmptyContent,
} from '@/components/ui/empty'
import { ProjectCard } from './project-card'
import { ImportProjectModal } from './import-project-modal'
import { useProjectStore, useProjectDataStore, importProjectAndSync } from '@/lib/store/projects'
import { useWorkspaceListStore } from '@/lib/store/workspace-list'
import { countReposByProject } from '@/lib/store/build-repo-tree'
import { useRetry } from '@/lib/store/use-retry'
import { DataState } from '@/components/ui/data-state'
import { dataOf } from '@/lib/loadable'
import type { Project } from '@/lib/types'

interface ProjectListPageProps {
  onSelect: (projectId: string) => void
}

export function ProjectListPage({ onSelect }: ProjectListPageProps) {
  const projectsLoadable = useProjectDataStore((s) => s.data)
  const reposLoadable = useWorkspaceListStore((s) => s.data)
  const repos = dataOf(reposLoadable) ?? []
  const repoCounts = countReposByProject(repos)
  const retry = useRetry(useProjectDataStore)
  const activeProjectId = useProjectStore((s) => s.activeProjectId)
  const setActiveProject = useProjectStore((s) => s.setActiveProject)
  const [importOpen, setImportOpen] = useState(false)

  const handleSelect = (id: string) => {
    setActiveProject(id)
    onSelect(id)
  }
  const handleImport = (project: Project) => {
    importProjectAndSync(project)
    setImportOpen(false)
  }

  return (
    <div className="flex flex-1 flex-col p-8">
      <DataState loadable={projectsLoadable} onRetry={retry} loadingLabel="Loading projects">
        {(projects) => {
          const hasRepos = repos.length > 0

          if (projects.length === 0) {
            return (
              <Empty>
                <EmptyHeader>
                  <EmptyTitle>No projects yet</EmptyTitle>
                  <EmptyDescription>Import a project folder to get started.</EmptyDescription>
                </EmptyHeader>
                <EmptyContent>
                  <Button className="w-full rounded-full" onClick={() => setImportOpen(true)}>
                    Import project
                  </Button>
                </EmptyContent>
              </Empty>
            )
          }

          if (!hasRepos && reposLoadable.status === 'success') {
            return (
              <Empty>
                <EmptyMedia variant="icon">
                  <GitBranchIcon />
                </EmptyMedia>
                <EmptyHeader>
                  <EmptyTitle>No repositories yet</EmptyTitle>
                  <EmptyDescription>
                    Add a git repository to open a workspace.
                  </EmptyDescription>
                </EmptyHeader>
                <EmptyContent>
                  <Button
                    variant="outline"
                    size="sm"
                    className="rounded-full"
                    onClick={() => setImportOpen(true)}
                  >
                    Add repository
                  </Button>
                </EmptyContent>
              </Empty>
            )
          }

          return (
            <>
              <div className="mb-6 flex items-center justify-between">
                <h1 className="text-xl font-semibold text-foreground">Projects</h1>
                <Button size="sm" onClick={() => setImportOpen(true)}>
                  + Import project
                </Button>
              </div>
              <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
                {projects.map((project) => (
                  <ProjectCard
                    key={project.id}
                    project={project}
                    active={project.id === activeProjectId}
                    repoCount={repoCounts.get(project.id) ?? 0}
                    onClick={() => handleSelect(project.id)}
                  />
                ))}
              </div>
            </>
          )
        }}
      </DataState>

      <ImportProjectModal open={importOpen} onOpenChange={setImportOpen} onImport={handleImport} />
    </div>
  )
}
