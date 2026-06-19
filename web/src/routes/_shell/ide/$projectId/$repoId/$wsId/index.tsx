import { createFileRoute } from '@tanstack/react-router'

export const Route = createFileRoute('/_shell/ide/$projectId/$repoId/$wsId/')({
  component: () => null,
})
