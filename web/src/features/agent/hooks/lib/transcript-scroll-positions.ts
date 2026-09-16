import type { TranscriptScrollPosition } from '@/features/agent/hooks/use-transcript-anchor'

/**
 * Where the reader last left each chat's transcript, keyed by chat id.
 *
 * A bare module-level map, deliberately NOT inside the per-workspace store
 * (`agent-chats-slice.ts`) or any other Zustand store: `destroyWorkspaceStore`
 * (workspace-store-registry.ts) drops that store from the registry wholesale
 * on every workspace switch, and `AgentChatsPanel` remounts on top of it —
 * so anything living there is gone the moment a chat's own workspace goes out
 * of view, which showed up live as the transcript sweeping from the top back
 * to the bottom on every return, exactly once per workspace round-trip
 * (switching chats WITHIN a workspace never destroys that workspace's store,
 * so it never reproduced there). This module survives a workspace switch
 * because nothing about it is workspace-scoped at all.
 *
 * In-memory only, deliberately never persisted to disk (unlike
 * agent-chat-order's localStorage): "still hot" means this running session,
 * not "restore across an app restart" — a cold app open has nothing more
 * useful to land on than the newest message anyway.
 */
const positions = new Map<string, TranscriptScrollPosition>()

export function getScrollPosition(chatId: string): TranscriptScrollPosition | null {
  return positions.get(chatId) ?? null
}

export function setScrollPosition(chatId: string, position: TranscriptScrollPosition): void {
  positions.set(chatId, position)
}

export function clearScrollPosition(chatId: string): void {
  positions.delete(chatId)
}

/** Test-only: this module is a singleton, so state otherwise leaks across
 *  `it()` blocks within a test file — same pattern as
 *  `lib/perf/instrumentation.ts`'s `__resetPerfForTests`. */
export function __resetScrollPositionsForTests(): void {
  positions.clear()
}
