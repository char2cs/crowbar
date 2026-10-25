import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { wsManager } from '@/lib/ws/manager'
import { listTabs, runAgentOp, type AgentRequest } from './agent-ops'
import { useBrowserAgentStore } from './agent-activity-store'

/** The daemon route the desktop app attaches to so agents can drive its tabs. */
const PEER_ENDPOINT = '/v0/browser/peer'

interface RequestFrame extends AgentRequest {
  type: 'request'
  id: string
}

function isRequest(frame: unknown): frame is RequestFrame {
  const f = frame as Partial<RequestFrame> | null
  return (
    f?.type === 'request' &&
    typeof f.id === 'string' &&
    typeof f.op === 'string' &&
    typeof f.chatId === 'string' &&
    f.chatId !== ''
  )
}

function messageOf(error: unknown): string {
  return error instanceof Error ? error.message : String(error)
}

async function answer(frame: RequestFrame): Promise<Record<string, unknown>> {
  const tab = typeof frame.tab === 'string' ? frame.tab : ''
  const { begin, end } = useBrowserAgentStore.getState()
  if (tab) begin(tab)
  try {
    const result = await runAgentOp({
      chatId: frame.chatId,
      op: frame.op,
      tab,
      args: frame.args ?? {},
    })
    return { type: 'response', id: frame.id, ok: true, result }
  } catch (error) {
    return { type: 'response', id: frame.id, ok: false, error: messageOf(error) }
  } finally {
    if (tab) end(tab)
  }
}

/**
 * Attaches this window to the daemon as the browser peer: reports its tabs
 * whenever they change and runs the requests agents send. The daemon speaks
 * first (`hello`) on every connection, so a reconnect re-reports without any
 * connection logic here.
 */
export function attachBrowserAgentPeer(): () => void {
  let sent = ''
  const publish = () => {
    const tabs = listTabs()
    const key = JSON.stringify(tabs)
    if (key === sent) return
    sent = key
    wsManager.send(PEER_ENDPOINT, { type: 'tabs', tabs })
  }
  const stopStore = windowPaneStore.subscribe(publish)
  const stopSocket = wsManager.subscribe(PEER_ENDPOINT, (frame) => {
    if ((frame as { type?: string } | null)?.type === 'hello') {
      sent = ''
      publish()
      return
    }
    if (!isRequest(frame)) return
    void answer(frame).then((reply) => {
      publish()
      wsManager.send(PEER_ENDPOINT, reply)
    })
  })
  return () => {
    stopStore()
    stopSocket()
  }
}
