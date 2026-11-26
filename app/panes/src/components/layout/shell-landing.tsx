import { Navigate, useNavigate } from '@tanstack/react-router'
import { useProjectStore } from '@/core/entities/active-project'
import { selectProjects, useSelect } from '@/core/entities/select'
import { useStore } from 'zustand'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { isChatRecalled } from '@/features/hosts/lib/host-resolve-cached'
import { HostProjectsPending } from '@/features/hosts/components/host-projects-pending'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import type { Project } from '@/lib/types'

type Landing = { to: '/add-host' } | { to: '/ide/$projectId/home'; params: { projectId: string } }

/** Where the app opens onto, given the project list; null while a paired host may still list
 *  one. The phone app has no daemon of its own, so it has no first run either. */
export function landingFor(projects: Project[], activeId: string): Landing | null {
  const project = projects.find((p) => p.id === activeId) ?? projects[0]
  if (project) return { to: '/ide/$projectId/home', params: { projectId: project.id } }
  return useHostConfigStore.getState().hosts.length === 0 ? { to: '/add-host' } : null
}

/** The landing until a project lands: it opens that project, or waits on the paired hosts. */
export function ShellLanding() {
  const projects = useSelect(selectProjects)
  const activeId = useProjectStore((s) => s.activeProjectId)
  // A chat the last run placed is already on screen; the panel is only for the empty stage.
  const chatShown = useStore(windowPaneStore, (s) =>
    Object.values(s.panes).some((p) => p.type === 'group' && p.chatId && isChatRecalled(p.chatId)),
  )
  const navigate = useNavigate()
  const landing = landingFor(projects, activeId)
  if (landing) return <Navigate {...landing} replace />
  if (chatShown) return null
  return (
    <HostProjectsPending
      onProject={(projectId) =>
        void navigate({ to: '/ide/$projectId/home', params: { projectId }, replace: true })
      }
      onAddHost={() => void navigate({ to: '/add-host' })}
    />
  )
}
