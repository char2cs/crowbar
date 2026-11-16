import { createWSManager } from '@/lib/ws/create-ws-manager'
import { hostPorts } from '@/features/hosts/lib/ws-host-ports'

/** The app's one socket manager, wired to the host layer. */
export const wsManager = createWSManager(hostPorts)
