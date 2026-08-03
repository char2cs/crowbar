import { useState } from 'react'
import { Plus } from 'lucide-react'
import { useNavigate } from '@tanstack/react-router'
import { cn } from '@/lib/utils'
import { ROW_BASE } from './workspace-row-base'
import { ImportProjectModal } from '@/components/projects/import-project-modal'
import { useSidebarNavStore } from '@/features/layout/stores/sidebar-nav'
import {
  useProjectStore,
  useProjectDataStore,
  importProjectAndSync,
  EMPTY_PROJECTS,
} from '@/lib/store/projects'
import { dataOf } from '@/lib/loadable'
import type { Project } from '@/lib/types'

/**
 * Body of the pushed "Projects" sidebar screen. NavStack supplies the
 * back button + title; this renders the project list and the import row.
 */
export function ProjectSwitcherPanel() {
  // Live project list (the import-only useProjectStore.projects starts empty and
  // only carries projects imported this session — see context-pill).
  const projects = useProjectDataStore((s) => dataOf(s.data) ?? EMPTY_PROJECTS)
  const activeProjectId = useProjectStore((s) => s.activeProjectId)
  const navigate = useNavigate()
  const [importOpen, setImportOpen] = useState(false)

  function handleSelect(id: string) {
    // Navigate to the selected project's home route. The route is the source of
    // truth for the displayed workspace/files; setActiveProject alone only moved
    // the context pill, leaving the route (and the file tree) on the old project.
    // ide-shell syncs activeProjectId from the route, but we set it here too so
    // the pill updates without waiting for the navigation effect.
    useProjectStore.getState().setActiveProject(id)
    void navigate({ to: '/ide/$projectId/home', params: { projectId: id } })
    useSidebarNavStore.getState().pop()
  }

  function handleImport(project: Project) {
    importProjectAndSync(project)
    setImportOpen(false)
  }

  return (
    <div className="flex h-full flex-col" data-oracle-id="project-switcher-panel">
      <div className="flex flex-col py-1">
        {projects.map((p, index) => {
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
              // Indexed rather than keyed off `p.id`: this list's length varies
              // (0..n projects, always plus the static Import row below), and
              // `ANCHORS.md` v1.8 refuses a capture where the same anchor id
              // appears twice under one root — the same fix
              // `sidebar-project-header.tsx`'s four buttons already needed, here
              // because the *count* rather than the identity of the rows varies.
              data-oracle-id={`project-switcher-panel-row-${index}`}
            >
              {/*
                `data-oracle-line-sized`, per `native/oracle/ANCHORS.md` v1.6 —
                the same reasoning as `git-row-name`: a blockified flex item
                (`items-center`, not `stretch`) with no explicit height takes
                its box from its own line box regardless of the row's own
                authored `h-9`. Indexed for the same reason the row's own id
                is (v1.8, see above).
              */}
              <span
                className="min-w-0 flex-1 truncate font-mono"
                data-oracle-id={`project-switcher-panel-row-${index}-label`}
                data-oracle-line-sized="true"
              >
                {p.name}
              </span>
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
          data-oracle-id="project-switcher-panel-import"
        >
          <Plus className="size-3.5 shrink-0" />
          <span
            className="min-w-0 flex-1 truncate text-left"
            data-oracle-id="project-switcher-panel-import-label"
            data-oracle-line-sized="true"
          >
            Import project
          </span>
        </button>
      </div>

      <ImportProjectModal open={importOpen} onOpenChange={setImportOpen} onImport={handleImport} />
    </div>
  )
}
