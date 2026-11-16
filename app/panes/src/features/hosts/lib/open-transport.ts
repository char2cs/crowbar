import { createTransport } from '@/lib/ws/create-transport'
import { hostPorts } from '@/features/hosts/lib/ws-host-ports'

/** Opens the live transport for `endpoint` on whichever host it names. */
export function openTransport(endpoint: string) {
  return createTransport(endpoint, hostPorts.isLocalHost)
}
