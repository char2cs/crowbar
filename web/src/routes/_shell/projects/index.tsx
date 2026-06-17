// web/src/routes/projects/index.tsx
import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { ProjectListPage } from '@/components/projects/project-list-page'

export const Route = createFileRoute('/_shell/projects/')({
  component: ProjectsPage,
})

function ProjectsPage() {
  const navigate = useNavigate()
  return <ProjectListPage onSelect={() => navigate({ to: '/' })} />
}
