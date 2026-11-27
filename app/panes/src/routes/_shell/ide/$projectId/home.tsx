import { createFileRoute } from '@tanstack/react-router'
import { ProjectHomeNotice } from '@/features/workspace/components/project-home-notice'

export const Route = createFileRoute('/_shell/ide/$projectId/home')({
  component: ProjectHomeNotice,
})
