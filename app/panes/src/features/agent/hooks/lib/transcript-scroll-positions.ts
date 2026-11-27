import type { TranscriptScrollPosition } from '@/features/agent/hooks/use-transcript-anchor'

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
