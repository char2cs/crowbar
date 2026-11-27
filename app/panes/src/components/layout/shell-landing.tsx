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

export function landingFor(projects: Project[], activeId: string): Landing | null {
  const project = projects.find((p) => p.id === activeId) ?? projects[0]
  if (project) return { to: '/ide/$projectId/home', params: { projectId: project.id } }
  return useHostConfigStore.getState().hosts.length === 0 ? { to: '/add-host' } : null
}

export function ShellLanding() {
  const projects = useSelect(selectProjects)
  const activeId = useProjectStore((s) => s.activeProjectId)
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
