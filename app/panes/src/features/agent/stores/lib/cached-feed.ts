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

export interface CachedFeed {
  hostId: string
  chatId: string
  schema: number
  rows: Rows
  cursor: FeedCursor
  savedAt: number
}
