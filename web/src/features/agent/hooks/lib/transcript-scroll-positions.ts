import type { TranscriptScrollPosition } from '@/features/agent/hooks/use-transcript-anchor'

/**
 * Where the reader last left each chat's transcript, keyed by chat id. Module
 * level so it survives a workspace's eviction; in memory only, since a cold
 * start has nothing better to land on than the newest message.
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
