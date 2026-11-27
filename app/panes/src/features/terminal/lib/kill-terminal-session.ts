import { terminalKill } from '@/lib/crowbar-bridge'
import { useTerminalStore } from '../stores/terminal-store'

export async function killTerminalSession(sessionId: string): Promise<void> {
  const store = useTerminalStore.getState()
  const connectionId = store.getSession(sessionId)?.connectionId
  store.removeSession(sessionId)
  if (connectionId) await terminalKill(connectionId)
}
