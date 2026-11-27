import { loadReconnect, clearReconnect } from '../lib/terminal-reconnect-map'

interface ResolveArgs {
  workspaceId: string
  tabSessionId: string
  storeConnectionId: string | undefined
  listLiveSessions: () => Promise<string[]>
  createTerminal: () => Promise<string>
  attachOnly?: boolean
}

export type ResolvedTerminal =
  { sessionId: string; created: boolean } | { gone: true } | { unknown: true }

async function listLive(list: () => Promise<string[]>): Promise<string[] | null> {
  try {
    return await list()
  } catch {
    return null
  }
}

export async function resolveTerminalSession(args: ResolveArgs): Promise<ResolvedTerminal> {
  const bound = args.attachOnly
    ? args.tabSessionId
    : (args.storeConnectionId ?? loadReconnect(args.workspaceId, args.tabSessionId))
  if (bound) {
    const live = await listLive(args.listLiveSessions)
    if (live === null) return { unknown: true }
    if (!live.includes(bound)) {
      clearReconnect(args.workspaceId, args.tabSessionId)
      return { gone: true }
    }
    return { sessionId: bound, created: false }
  }
  return { sessionId: await args.createTerminal(), created: true }
}
