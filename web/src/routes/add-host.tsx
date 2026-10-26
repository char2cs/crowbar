import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { AddHostScreen } from '@/features/hosts/components/add-host-screen'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'

export const Route = createFileRoute('/add-host')({
  component: AddHostRoute,
})

function AddHostRoute() {
  const navigate = useNavigate()
  const hasHost = useHostConfigStore((s) => s.hosts.length > 0)
  const toLanding = () => void navigate({ to: '/' })
  return <AddHostScreen onPaired={toLanding} onCancel={hasHost ? toLanding : undefined} />
}
