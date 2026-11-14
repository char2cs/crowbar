import type { TranscriptItem } from '@/features/agent/stores/lib/transcript-items'

/** A long chat holds tens of thousands of items and a live row changes one or
 *  two: copying them all per row cost a frame, so a write copies one bucket. */
const BUCKETS = 64
type Bucket = Map<string, TranscriptItem>

function bucketOf(id: string): number {
  let hash = 0
  for (let i = 0; i < id.length; i++) hash = (Math.imul(hash, 31) + id.charCodeAt(i)) | 0
  return (hash >>> 0) % BUCKETS
}

/** A chat's items by id, immutable: an edit shares every bucket it did not touch. */
export class ItemMap {
  static readonly empty = new ItemMap(Array.from({ length: BUCKETS }, () => new Map()))

  private constructor(private readonly buckets: readonly Bucket[]) {}

  get = (id: string) => this.buckets[bucketOf(id)]!.get(id)
  has = (id: string) => this.buckets[bucketOf(id)]!.has(id)

  /** `finish` is the edited map (this one when nothing was written); a replay
   *  writes on after it, into the map it gave. */
  edit() {
    let buckets: Bucket[] | null = null
    let finished: ItemMap | null = null
    const owned = new Set<number>()
    const read = (at: number) => (buckets ?? this.buckets)[at]!
    const writable = (at: number) => {
      buckets ??= [...this.buckets]
      if (owned.has(at)) return buckets[at]!
      owned.add(at)
      buckets[at] = new Map(buckets[at])
      return buckets[at]!
    }
    return {
      get: (id: string) => read(bucketOf(id)).get(id),
      set: (item: TranscriptItem) => void writable(bucketOf(item.id)).set(item.id, item),
      delete: (id: string) => {
        if (read(bucketOf(id)).has(id)) writable(bucketOf(id)).delete(id)
      },
      finish: (): ItemMap => (buckets ? (finished ??= new ItemMap(buckets)) : this),
    }
  }
}

export type ItemDraft = ReturnType<ItemMap['edit']>
