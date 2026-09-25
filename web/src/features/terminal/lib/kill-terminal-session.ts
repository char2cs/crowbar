import { terminalKill } from '@/lib/crowbar-bridge'
import { useTerminalStore } from '../stores/terminal-store'

// Kill the backend PTY for a terminal tab that was closed for good. Closing
// the tab is the only owner of the session: terminal buffers are never added
// to the undo-close history, so the shell process must die here or it leaks
// (BUG-015). Looks up the tab's daemon session in the terminal store, drops the
// store entry, then DELETEs the session (its views see the exit frame and
// detach; the view's own unmount closes its transport).
export async function killTerminalSession(sessionId: string): Promise<void> {
  const store = useTerminalStore.getState()
  const connectionId = store.getSession(sessionId)?.connectionId
  store.removeSession(sessionId)
  if (connectionId) await terminalKill(connectionId)
}
