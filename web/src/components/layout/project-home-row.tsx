import { House } from '@phosphor-icons/react'
import { useNavigate, useMatch } from '@tanstack/react-router'
import { cn } from '@/lib/utils'
import { ROW_BASE, ROW_ACTIVE, ROW_INACTIVE } from './workspace-row-base'
import { useProjectStore } from '@/lib/store/projects'

export function ProjectHomeRow() {
  const navigate = useNavigate()
  const projectId = useProjectStore((s) => s.activeProjectId)
  const isActive = useMatch({ from: '/ide/$projectId/home', shouldThrow: false })

  function handleClick() {
    if (!projectId) return
    void navigate({ to: '/ide/$projectId/home', params: { projectId } })
  }

  return (
    <div
      role="button"
      tabIndex={0}
      className={cn(ROW_BASE, isActive ? ROW_ACTIVE : ROW_INACTIVE)}
      onClick={handleClick}
      onKeyDown={(e) => {
        if (e.key === 'Enter' || e.key === ' ') handleClick()
      }}
    >
      <House size={14} weight={isActive ? 'fill' : 'regular'} />
      <span>Home</span>
    </div>
  )
}
