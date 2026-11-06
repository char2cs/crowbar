import type { TranscriptItem } from '@/features/agent/stores/lib/transcript-items'

/** A long chat holds tens of thousands of items and a live row changes one or
 *  two: copying them all per row cost a frame, so a write copies one bucket. */
const BUCKETS = 64

type Bucket = ReadonlyMap<string, TranscriptItem>

const NO_ITEMS: Bucket = new Map()

function bucketOf(id: string): number {
  let hash = 0
  for (let i = 0; i < id.length; i++) hash = (Math.imul(hash, 31) + id.charCodeAt(i)) | 0
  return (hash >>> 0) % BUCKETS
}

/** Writes against an ItemMap; `finish` is the edited map, or the same one when
 *  nothing was written. A replay writes on after `finish`, into the map it gave. */
export interface ItemDraft {
  get(id: string): TranscriptItem | undefined
  set(item: TranscriptItem): void
  delete(id: string): void
  finish(): ItemMap
}

/** A chat's items by id, immutable: an edit shares every bucket it did not touch. */
export class ItemMap {
  static readonly empty = new ItemMap(Array.from({ length: BUCKETS }, () => NO_ITEMS))

  private constructor(private readonly buckets: readonly Bucket[]) {}

  get(id: string): TranscriptItem | undefined {
    return this.buckets[bucketOf(id)]!.get(id)
  }

  has(id: string): boolean {
    return this.buckets[bucketOf(id)]!.has(id)
  }

  /** Each bucket the edits touch is copied once, however many items they write. */
  edit(): ItemDraft {
    let buckets: Bucket[] | null = null
    let finished: ItemMap | null = null
    const owned = new Set<number>()
    const writable = (at: number) => {
      buckets ??= [...this.buckets]
      if (!owned.has(at)) {
        owned.add(at)
        buckets[at] = new Map(buckets[at])
      }
      return buckets[at] as Map<string, TranscriptItem>
    }
    return {
      get: (id) => (buckets ?? this.buckets)[bucketOf(id)]!.get(id),
      set: (item) => void writable(bucketOf(item.id)).set(item.id, item),
      delete: (id) => {
        const at = bucketOf(id)
        if ((buckets ?? this.buckets)[at]!.has(id)) writable(at).delete(id)
      },
      finish: () => (buckets ? (finished ??= new ItemMap(buckets)) : this),
    }
  }
}
