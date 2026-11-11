import { terminalKill } from '@/lib/crowbar-bridge'
import { useTerminalStore } from '../stores/terminal-store'

// Closing a tab for good is the session's only owner (terminal buffers never enter undo-close), so
// the shell must die here or leak. Views detach on the exit frame and close their own transports.
export async function killTerminalSession(sessionId: string): Promise<void> {
  const store = useTerminalStore.getState()
  const connectionId = store.getSession(sessionId)?.connectionId
  store.removeSession(sessionId)
  if (connectionId) await terminalKill(connectionId)
}
