export interface WorkspaceEvent {
  workspaceId: string
  action: string
}
export interface GitEvent {
  repo: string
  changed: boolean
}
export interface FileEvent {
  workspaceId: string
  path: string
}
export interface ChatChunk {
  chatId: string
  content: string
  done: boolean
}
export interface TerminalFrame {
  sessionId: string
  data: string
}
export interface DaemonStatus {
  status: string
  version?: string
}

// ---------------------------------------------------------------------------
// §6 entity-stream frames. Under the hierarchical model the WS no longer pushes
// thin {workspaceId,action} envelopes — each frame is a COMPLETE DTO (matching
// the GET shape), upserted into the entity cache by its own `id`. A frame whose
// `status` is "deleted" is a tombstone that removes the entity.
//
// The reconnect sentinel ({reconnected:true}) is the one non-DTO frame on these
// channels: it carries no `id` and must trigger a full GET reseed, never an
// upsert. (Raw PTY frames stay TerminalFrame above.)
// ---------------------------------------------------------------------------

/** A complete-DTO WS frame: at minimum an `id`, optionally a `status`. */
export interface EntityFrame {
  id: string
  status?: string
}

/** The reconnect sentinel pushed by wsManager after a dropped/restored socket. */
export interface ReconnectSentinel {
  reconnected: true
}

export function isReconnectSentinel(data: unknown): data is ReconnectSentinel {
  return (
    typeof data === 'object' &&
    data !== null &&
    (data as { reconnected?: unknown }).reconnected === true
  )
}
