import { useState } from 'react'
import { Plus } from 'lucide-react'
import { cn } from '@/lib/utils'
import { ROW_BASE } from './workspace-row-base'
import { ImportProjectModal } from '@/components/projects/import-project-modal'
import { useSidebarNavStore } from '@/features/layout/stores/sidebar-nav'
import { useProjectStore, importProjectAndSync } from '@/lib/store/projects'
import type { Project } from '@/lib/types'

/**
 * Body of the pushed "Projects" sidebar screen. NavStack supplies the
 * back button + title; this renders the project list and the import row.
 */
export function ProjectSwitcherPanel() {
  const projects = useProjectStore((s) => s.projects)
  const activeProjectId = useProjectStore((s) => s.activeProjectId)
  const [importOpen, setImportOpen] = useState(false)

  function handleSelect(id: string) {
    useProjectStore.getState().setActiveProject(id)
    useSidebarNavStore.getState().pop()
  }

  function handleImport(project: Project) {
    importProjectAndSync(project)
    setImportOpen(false)
  }

  return (
    <div className="flex h-full flex-col">
      <div className="flex flex-col py-1">
        {projects.map((p) => {
          const isActive = p.id === activeProjectId
          return (
            <button
              key={p.id}
              type="button"
              aria-current={isActive ? 'true' : 'false'}
              onClick={() => handleSelect(p.id)}
              className={cn(
                ROW_BASE,
                'border-transparent text-left hover:bg-accent',
                isActive && 'bg-accent/60 text-foreground',
              )}
            >
              <span className="min-w-0 flex-1 truncate font-mono">{p.name}</span>
            </button>
          )
        })}

        <button
          type="button"
          onClick={() => setImportOpen(true)}
          className={cn(
            ROW_BASE,
            'border-transparent text-muted-foreground hover:bg-accent hover:text-foreground',
          )}
        >
          <Plus className="size-3.5 shrink-0" />
          <span className="min-w-0 flex-1 truncate text-left">Import project</span>
        </button>
      </div>

      <ImportProjectModal
        open={importOpen}
        onOpenChange={setImportOpen}
        onImport={handleImport}
      />
    </div>
  )
}
