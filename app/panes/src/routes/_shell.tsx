import { createFileRoute } from '@tanstack/react-router'
import { MobileShell } from '@/components/layout/mobile/mobile-shell'

export const Route = createFileRoute('/_shell')({
  component: MobileShell,
})
