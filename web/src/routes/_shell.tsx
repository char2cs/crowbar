import { createFileRoute } from '@tanstack/react-router'
import { IDEShell } from '@/components/layout/ide-shell'

export const Route = createFileRoute('/_shell')({
  component: IDEShell,
})
