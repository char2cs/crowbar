import type { ChatFeed, TranscriptItem } from '@/features/agent/stores/lib/transcript-items'

type Rows = Omit<
  ChatFeed,
  'status' | 'error' | 'epoch' | 'seq' | 'from' | 'items' | 'liveTail' | 'live' | 'liveN'
> & { items: TranscriptItem[] }

interface FeedCursor {
  seq: number
  from: number
  epoch: string
}

/** One chat's folded feed as the IndexedDB feed cache stores it, by host and chat. */
export interface CachedFeed {
  hostId: string
  chatId: string
  /** The cache's schema when written: bumped when ChatFeed's folded shape changes, so an entry
   *  of another schema is dropped on read. */
  schema: number
  /** The folded feed minus what a reload cannot trust: live text, status and cursor. */
  rows: Rows
  /** Where the feed resumes: the ledger run, the last folded row, and the first one held. */
  cursor: FeedCursor
  savedAt: number
}
