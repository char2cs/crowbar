import { createFileRoute, redirect } from '@tanstack/react-router'
import { OobeScreen } from '@/components/oobe/oobe-screen'
import { hasLocalDaemon } from '@/features/hosts/lib/local-daemon'

export const Route = createFileRoute('/oobe')({
  // OOBE sets up this machine's own daemon; an install without one connects to a host instead.
  beforeLoad: () => {
    if (!hasLocalDaemon()) throw redirect({ to: '/add-host' })
  },
  component: OobeScreen,
})
