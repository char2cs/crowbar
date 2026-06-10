import { useState } from 'react'
import { Button } from '@/components/ui/button'
import { ProjectCard } from './project-card'
import { ImportProjectModal } from './import-project-modal'
import { useProjectStore, useProjectDataStore } from '@/lib/store/projects'
import { useRetry } from '@/lib/store/use-retry'
import { DataState } from '@/components/ui/data-state'
import type { Project } from '@/lib/types'

interface ProjectListPageProps {
  onSelect: (projectId: string) => void
}

export function ProjectListPage({ onSelect }: ProjectListPageProps) {
  const projectsLoadable = useProjectDataStore((s) => s.data)
  const retry = useRetry(useProjectDataStore)
  const activeProjectId = useProjectStore((s) => s.activeProjectId)
  const setActiveProject = useProjectStore((s) => s.setActiveProject)
  const addProject = useProjectStore((s) => s.addProject)
  const [importOpen, setImportOpen] = useState(false)

  const handleSelect = (id: string) => {
    setActiveProject(id)
    onSelect(id)
  }
  const handleImport = (project: Project) => {
    addProject(project)
    setImportOpen(false)
    void useProjectDataStore.getState().fetch()
  }

  return (
    <div className="flex flex-1 flex-col p-8">
      <div className="mb-6 flex items-center justify-between">
        <h1 className="text-xl font-semibold text-foreground">Projects</h1>
        <Button size="sm" onClick={() => setImportOpen(true)}>
          + Import project
        </Button>
      </div>

      <DataState loadable={projectsLoadable} onRetry={retry} loadingLabel="Loading projects">
        {(projects) =>
          projects.length === 0 ? (
            <div className="flex flex-1 flex-col items-center justify-center gap-3 text-center">
              <p className="text-lg font-medium text-foreground">No projects yet</p>
              <p className="text-sm text-muted-foreground">
                Import a local project folder to get started.
              </p>
              <Button onClick={() => setImportOpen(true)}>Import project</Button>
            </div>
          ) : (
            <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
              {projects.map((project) => (
                <ProjectCard
                  key={project.id}
                  project={project}
                  active={project.id === activeProjectId}
                  repoCount={3}
                  onClick={() => handleSelect(project.id)}
                />
              ))}
            </div>
          )
        }
      </DataState>

      <ImportProjectModal open={importOpen} onOpenChange={setImportOpen} onImport={handleImport} />
    </div>
  )
}
