import { terminalDetach } from '@/lib/crowbar-bridge'
import { useTerminalStore } from '../stores/terminal-store'
import { saveReconnect } from './terminal-reconnect-map'

// Workspace-switch teardown for a pane terminal: keep the daemon PTY alive,
// just close the WS transport. The in-memory store entry is kept so re-entry
// can reuse the connectionId; a localStorage mirror is the reload backstop.
export async function detachTerminalSession(
  workspaceId: string,
  tabSessionId: string,
): Promise<void> {
  const connectionId = useTerminalStore.getState().getSession(tabSessionId)?.connectionId
  if (!connectionId) return
  saveReconnect(workspaceId, tabSessionId, connectionId)
  await terminalDetach(connectionId)
}
