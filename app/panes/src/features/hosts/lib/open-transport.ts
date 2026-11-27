import { createTransport } from '@/lib/ws/create-transport'
import { hostPorts } from '@/features/hosts/lib/ws-host-ports'

export function openTransport(endpoint: string) {
  return createTransport(endpoint, hostPorts.isLocalHost)
}
