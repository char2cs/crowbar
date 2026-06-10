// web/src/routes/projects/index.tsx
import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { ProjectListPage } from '@/components/projects/ProjectListPage'

export const Route = createFileRoute('/projects/')({
  component: ProjectsPage,
})

function ProjectsPage() {
  const navigate = useNavigate()
  return <ProjectListPage onSelect={() => navigate({ to: '/' })} />
}
