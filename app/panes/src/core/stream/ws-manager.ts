import { createWSManager } from '@/lib/ws/create-ws-manager'
import { hostPorts } from '@/features/hosts/lib/ws-host-ports'

export const wsManager = createWSManager(hostPorts)
