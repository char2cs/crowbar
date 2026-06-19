import { terminalClose } from '@/lib/crowbar-bridge'
import { useTerminalStore } from '../stores/terminal-store'

// Kill the backend PTY for a terminal tab that was closed for good. Closing
// the tab is the only owner of the session: terminal buffers are never added
// to the undo-close history, so the shell process must die here or it leaks
// (BUG-015). Looks up the live connection id in the terminal store, drops the
// session entry, then closes the WS and DELETEs the hierarchical
// /v0/projects/:p/repos/:r/workspaces/:w/terminals/:sessionId route (resolved
// from the recorded per-session scope inside terminalClose).
export async function killTerminalSession(sessionId: string): Promise<void> {
  const store = useTerminalStore.getState()
  const connectionId = store.getSession(sessionId)?.connectionId
  store.removeSession(sessionId)
  if (connectionId) await terminalClose(connectionId)
}
