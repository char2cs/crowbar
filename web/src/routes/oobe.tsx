import { createFileRoute } from '@tanstack/react-router'
import { OobeScreen } from '@/components/oobe/oobe-screen'

export const Route = createFileRoute('/oobe')({
  component: OobeScreen,
})
