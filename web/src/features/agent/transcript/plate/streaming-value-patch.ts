import type { DecoratedRange } from '@platejs/slate'
import type { NodeEntry, Path, TText, Value } from 'platejs'
import type { PlateEditor } from 'platejs/react'
import {
  commonPrefixLength,
  commonSuffixLength,
  flattenText,
  lastLeaf,
  leafDivergences,
  markKeys,
  nodesEqual,
  stableBlockCount,
  trailingTextDivergence,
  type LeafDiff,
  type Node,
} from '@/features/agent/transcript/plate/streaming-value-blocks'

export {
  stableBlockCount,
  trailingTextDivergence,
  type TextDivergence,
} from '@/features/agent/transcript/plate/streaming-value-blocks'

/** The fade-in mark, only ever a Slate decoration — never in the document. */
export const CHAT_FRESH_MARK = 'chatFresh'
/** This leaf's `animation-delay` in ms. Constant per run: a changing value
 *  defeats `isTextDecorationsEqual` and rewrites a running animation's delay;
 *  elapsed time is applied once at mount by `resumedFadeDelay`. */
export const CHAT_FRESH_DELAY_MARK = 'chatFreshDelay'
/** When the run landed (`performance.now()`), so a remount can resume its fade. */
export const CHAT_FRESH_BORN_MARK = 'chatFreshBornAt'
/** A finished fade kept only to hold the leaf split stable for later words;
 *  matches no plugin, so it renders as plain text with no animation to restart. */
export const CHAT_FRESH_HELD = 'chatFreshHeld'
/** Per-word index and generation total, only on a per-word split, so each word's
 *  `animationend` counts on its own instead of the first one settling the run. */
export const CHAT_FRESH_WORD_INDEX_MARK = 'chatFreshWordIndex'
export const CHAT_FRESH_WORD_TOTAL_MARK = 'chatFreshWordTotal'

// The step scales down for a long chunk instead of the delay capping, so every
// word keeps a distinct delay and the cascade still ends within MAX_STAGGER_MS.
const WORD_STAGGER_MS = 30
const MAX_STAGGER_MS = 320
export function staggerDelay(index: number, total: number): number {
  const step = total <= 1 ? 0 : Math.min(WORD_STAGGER_MS, MAX_STAGGER_MS / (total - 1))
  return index * step
}

// Flat head start per word so scroll-follow gets underway before text appears.
const SCROLL_LEAD_MS = 150

// Past this many words the stagger step is imperceptible, so the chunk fades
// as one span instead of paying for hundreds.
const WORD_SPLIT_CAP = 80

// How long `chat-token-fade` itself runs — keep in step with transcript.css.
const FADE_DURATION_MS = 260

// One frame, the resolution `resumedFadeDelay` measures a run's age in.
const FRAME_MS = 16

/** Whitespace-preserving split: joining the result reconstructs the input. */
export function splitIntoWords(text: string): string[] {
  return text.match(/\s*\S+\s*/g) ?? [text]
}

/** One insertion's just-arrived text, as a range over text the document
 *  already holds — the fade is a decoration, never a document change. */
interface FreshRun {
  /** Distinct per insertion so adjacent runs keep their own timing. */
  generation: number
  path: Path
  start: number
  end: number
  /** One insertion can span several leaves; these keep the cascade continuous. */
  wordOffset: number
  totalWords: number
  /** The fade plays against this clock, not the span's life: slate-react
   *  remounts these leaves constantly, and without it every remount would
   *  restart from invisible. Stamped by `recordRun`. */
  bornAt: number
}

// Decorations, not per-word leaves in the document: a chunk is one
// `insert_text` regardless of word count, and settling costs no operation.
const freshRuns = new WeakMap<PlateEditor, FreshRun[]>()
const settledGenerations = new WeakMap<PlateEditor, Set<number>>()
const freshGenerations = new WeakMap<PlateEditor, number>()

// Backstop for runs whose `animationend` never fires (reduced motion, hidden
// chat); ~1s of history at one flush per frame, long after any visible fade.
const MAX_LIVE_GENERATIONS = 60

function nextFreshGeneration(editor: PlateEditor): number {
  const next = (freshGenerations.get(editor) ?? 0) + 1
  freshGenerations.set(editor, next)
  return next
}

function pathEquals(a: Path, b: Path): boolean {
  return a.length === b.length && a.every((step, i) => step === b[i])
}

function recordRun(editor: PlateEditor, run: Omit<FreshRun, 'bornAt'>): void {
  const runs = freshRuns.get(editor) ?? []
  runs.push({
    ...run,
    bornAt: typeof performance === 'object' ? performance.now() : Date.now(),
  })
  freshRuns.set(editor, runs)
}

const pathKey = (path: Path) => path.join('.')

/**
 * Drops unrenderable runs, but keeps a settled run while any run in its text
 * node still fades: slate-react keys leaves by split index, so dropping one
 * would remount (and restart) every fading leaf after it.
 */
function pruneRuns(editor: PlateEditor, invalidFromBlock: number): void {
  const runs = freshRuns.get(editor)
  if (!runs?.length) return
  const settled = settledGenerations.get(editor)
  const floor = (freshGenerations.get(editor) ?? 0) - MAX_LIVE_GENERATIONS
  const live = (run: FreshRun) => !settled?.has(run.generation) && run.generation > floor
  const animating = new Set<string>()
  for (const run of runs) if (live(run)) animating.add(pathKey(run.path))
  const kept = runs.filter((run) => {
    if ((run.path[0] ?? 0) >= invalidFromBlock) return false
    if (live(run)) return true
    return animating.has(pathKey(run.path))
  })
  freshRuns.set(editor, kept)
  if (settled && kept.length === 0) settled.clear()
}

// One redecorate pass per frame, however many words finished in it.
const pendingCleanup = new WeakMap<PlateEditor, number>()

function scheduleFadeCleanup(editor: PlateEditor): void {
  if (typeof requestAnimationFrame !== 'function') return
  if (pendingCleanup.has(editor)) return
  pendingCleanup.set(
    editor,
    requestAnimationFrame(() => {
      pendingCleanup.delete(editor)
      // Absent on a headless editor, which has no rendering to invalidate.
      editor.api.redecorate?.()
    }),
  )
}

/**
 * Retires one finished fade, from the real `animationend`. Schedules a
 * redecorate because only re-rendered blocks recompute decorations, and a
 * streaming list re-renders only its last item.
 */
export function settleFreshGeneration(editor: PlateEditor, generation: number): void {
  const settled = settledGenerations.get(editor) ?? new Set<number>()
  if (settled.has(generation)) return
  settled.add(generation)
  settledGenerations.set(editor, settled)
  scheduleFadeCleanup(editor)
}

const settledWordCounts = new WeakMap<PlateEditor, Map<number, Set<number>>>()

/**
 * One word of a per-word cascade finished; the generation settles only once
 * every word has reported, since word 0 always finishes first.
 */
export function settleFreshWord(
  editor: PlateEditor,
  generation: number,
  wordIndex: number,
  totalWords: number,
): void {
  const byGeneration = settledWordCounts.get(editor) ?? new Map<number, Set<number>>()
  const words = byGeneration.get(generation) ?? new Set<number>()
  // A Set, not a counter: a remounted leaf can replay the same word's
  // `animationend`, and a repeat must not over-count.
  words.add(wordIndex)
  byGeneration.set(generation, words)
  settledWordCounts.set(editor, byGeneration)
  if (words.size >= totalWords) settleFreshGeneration(editor, generation)
}

/** The fade as per-word Slate ranges over `entry`'s leaf; Slate splits the leaf
 *  along them at render, so the document never holds a leaf per word. */
export function freshDecorations(editor: PlateEditor, [node, path]: NodeEntry): DecoratedRange[] {
  const text = (node as Node).text
  if (typeof text !== 'string') return []
  const runs = freshRuns.get(editor)
  if (!runs?.length) return []
  const settled = settledGenerations.get(editor)

  const floor = (freshGenerations.get(editor) ?? 0) - MAX_LIVE_GENERATIONS

  const ranges: DecoratedRange[] = []
  for (const run of runs) {
    if (!pathEquals(run.path, path)) continue
    // A reparse can reshape the leaf; offsets naming text no longer there are dropped.
    if (run.end > text.length || run.start >= run.end) continue
    // Not time-based: retiring on a clock would swap the span for a plain one
    // and re-render on-screen text. Stranded fades are handled by `resumedFadeDelay`.
    const held = settled?.has(run.generation) || run.generation <= floor
    const mark = (offset: number, next: number, delay: number, wordIndex?: number) =>
      ranges.push(
        (held
          ? { anchor: { path, offset }, focus: { path, offset: next }, [CHAT_FRESH_HELD]: true }
          : {
              anchor: { path, offset },
              focus: { path, offset: next },
              [CHAT_FRESH_MARK]: run.generation,
              [CHAT_FRESH_DELAY_MARK]: delay,
              [CHAT_FRESH_BORN_MARK]: run.bornAt,
              ...(wordIndex === undefined
                ? {}
                : {
                    [CHAT_FRESH_WORD_INDEX_MARK]: wordIndex,
                    [CHAT_FRESH_WORD_TOTAL_MARK]: run.totalWords,
                  }),
            }) as unknown as DecoratedRange,
      )

    // One unsplit fade, settled directly (no word index).
    if (run.totalWords > WORD_SPLIT_CAP) {
      mark(run.start, run.end, SCROLL_LEAD_MS)
      continue
    }
    let offset = run.start
    splitIntoWords(text.slice(run.start, run.end)).forEach((word, i) => {
      const next = offset + word.length
      const wordIndex = run.wordOffset + i
      mark(offset, next, SCROLL_LEAD_MS + staggerDelay(wordIndex, run.totalWords), wordIndex)
      offset = next
    })
  }
  return ranges
}

// Leading blocks already confirmed stable; never re-compared, which keeps a
// streamed message from costing O(length²).
const knownStablePrefix = new WeakMap<PlateEditor, number>()

/** Patches leaf marks and text in place — never removing/inserting a node — so
 *  every ancestor keeps its reference and `NodeIdPlugin` id. */
function patchLeavesInPlace(editor: PlateEditor, diffs: LeafDiff[]): void {
  for (const { path, prev, next } of diffs) {
    const prevKeys = markKeys(prev)
    const nextKeys = markKeys(next)
    const toUnset = prevKeys.filter((k) => !(k in next))
    if (toUnset.length) editor.tf.unsetNodes(toUnset, { at: path })
    const toSet: Record<string, unknown> = {}
    for (const k of nextKeys) {
      if (
        !(k in prev) ||
        !nodesEqual((prev as Record<string, unknown>)[k], (next as Record<string, unknown>)[k])
      ) {
        toSet[k] = (next as Record<string, unknown>)[k]
      }
    }
    if (Object.keys(toSet).length) editor.tf.setNodes(toSet, { at: path })

    const prevText = (prev.text as string) ?? ''
    const nextText = (next.text as string) ?? ''
    if (prevText === nextText) continue
    if (prevText.length) {
      editor.tf.delete({ at: { path, offset: 0 }, distance: prevText.length, unit: 'character' })
    }
    if (nextText.length) {
      editor.tf.insertText(nextText, { at: { path, offset: 0 } })
      // The whole leaf: its old text went the moment its marks changed.
      recordRun(editor, {
        generation: nextFreshGeneration(editor),
        path,
        start: 0,
        end: nextText.length,
        wordOffset: 0,
        totalWords: splitIntoWords(nextText).length,
      })
    }
  }
}

/** Records a fade over every leaf of an inserted block with one continuous word
 *  index. `skip` (shared, mutated) is flattened text already on screen, so a
 *  replaced block does not re-fade words the reader has already seen. */
function recordBlockRuns(
  editor: PlateEditor,
  node: Node,
  path: Path,
  generation: number,
  wordIndex: { current: number },
  total: number,
  // `take` bounds the fresh span after the skip; unset means "to the block's end".
  skip: { remaining: number; take?: number } = { remaining: 0 },
): void {
  if (typeof node.text === 'string') {
    const length = node.text.length
    if (length === 0) return
    if (skip.remaining >= length) {
      skip.remaining -= length
      return
    }
    const take = skip.take ?? Number.POSITIVE_INFINITY
    if (take <= 0) return
    const start = skip.remaining
    skip.remaining = 0
    const end = Math.min(length, start + take)
    if (skip.take !== undefined) skip.take -= end - start
    if (end <= start) return
    recordRun(editor, {
      generation,
      path,
      start,
      end,
      wordOffset: wordIndex.current,
      totalWords: total,
    })
    wordIndex.current += splitIntoWords(node.text.slice(start, end)).length
    return
  }
  node.children?.forEach((child, i) => {
    recordBlockRuns(editor, child, [...path, i], generation, wordIndex, total, skip)
  })
}

/**
 * Applies `next` touching only the blocks that differ — `setValue` would tear
 * down and rebuild every settled paragraph per token. A growing tail becomes
 * one `insert_text`, and the fade is recorded as runs, not document marks.
 */
export function applyStreamedValue(editor: PlateEditor, next: Value): void {
  const prev = editor.children as Value
  const startAt = Math.min(knownStablePrefix.get(editor) ?? 0, prev.length, next.length)
  const stable = startAt + stableBlockCount(prev.slice(startAt), next.slice(startAt))
  // Never latch the last block: it is still being written, and a delta that
  // reparses identically (e.g. mid table separator row) would freeze it forever.
  knownStablePrefix.set(editor, Math.min(stable, Math.max(next.length - 1, 0)))
  if (stable === prev.length && stable === next.length) return

  editor.tf.withoutNormalizing(() => {
    if (stable === prev.length - 1 && stable === next.length - 1) {
      const prevBlock = prev[stable] as Node
      const nextBlock = next[stable] as Node
      const divergence = trailingTextDivergence(prevBlock, nextBlock)
      if (divergence) {
        const prevText = lastLeaf(prevBlock).text as string
        const staleLength = prevText.length - divergence.keep
        if (staleLength === 0 && divergence.replacement === '') return // truly unchanged
        const endPoint = editor.api.end([stable])
        if (endPoint) {
          // Delete only the diverging suffix so the shared prefix keeps its fade state.
          if (staleLength > 0) {
            // Offsets in this block are about to shift, so its runs go with it.
            pruneRuns(editor, stable)
            editor.tf.delete({
              at: endPoint,
              distance: staleLength,
              unit: 'character',
              reverse: true,
            })
          } else {
            pruneRuns(editor, Number.POSITIVE_INFINITY)
          }
          if (divergence.replacement !== '') {
            // Re-read: the delete shifted the block's end.
            const insertAt = staleLength > 0 ? editor.api.end([stable]) : endPoint
            if (insertAt) {
              editor.tf.insertText(divergence.replacement, { at: insertAt })
              recordRun(editor, {
                generation: nextFreshGeneration(editor),
                path: insertAt.path,
                start: insertAt.offset,
                end: insertAt.offset + divergence.replacement.length,
                wordOffset: 0,
                totalWords: splitIntoWords(divergence.replacement).length,
              })
            }
          }
          return
        }
      }
      if (!divergence) {
        // Usually a mark completing mid-paragraph. Patch leaves in place when the
        // shape is unchanged: a block replace reassigns its NodeIdPlugin id,
        // remounting the block and restarting its fade.
        const leafDiffs: LeafDiff[] = []
        if (leafDivergences(prevBlock, nextBlock, [stable], leafDiffs) && leafDiffs.length > 0) {
          pruneRuns(editor, stable)
          patchLeavesInPlace(editor, leafDiffs)
          return
        }
        // Shape changed: replace the block, fading only the genuinely new text.
        pruneRuns(editor, stable)
        editor.tf.removeNodes({ at: [stable] })
        editor.tf.insertNodes([nextBlock] as Value, { at: [stable] })
        const prevText = flattenText(prevBlock)
        const nextText = flattenText(nextBlock)
        const keepPrefix = commonPrefixLength(prevText, nextText)
        const keepSuffix = Math.min(
          commonSuffixLength(prevText, nextText),
          nextText.length - keepPrefix,
        )
        const freshSuffix = nextText.slice(keepPrefix, nextText.length - keepSuffix)
        if (freshSuffix !== '') {
          recordBlockRuns(
            editor,
            nextBlock,
            [stable],
            nextFreshGeneration(editor),
            { current: 0 },
            splitIntoWords(freshSuffix).length,
            { remaining: keepPrefix, take: freshSuffix.length },
          )
        }
        return
      }
    }
    pruneRuns(editor, stable)
    // Text carried over verbatim is already on screen; skip it so one flush that
    // finishes a paragraph and starts the next does not re-fade the first.
    const carriedOver = commonPrefixLength(
      prev
        .slice(stable)
        .map((n) => flattenText(n as Node))
        .join(''),
      next
        .slice(stable)
        .map((n) => flattenText(n as Node))
        .join(''),
    )
    let removing = prev.length - stable
    while (removing-- > 0) editor.tf.removeNodes({ at: [stable] })
    if (stable < next.length) {
      const newNodes = next.slice(stable)
      const generation = nextFreshGeneration(editor)
      const wordIndex = { current: 0 }
      const freshText = next
        .slice(stable)
        .map((n) => flattenText(n as Node))
        .join('')
        .slice(carriedOver)
      const total = splitIntoWords(freshText).length
      editor.tf.insertNodes(newNodes as Value, { at: [stable] })
      // One skip counter across all blocks, consumed across block boundaries.
      const skip = { remaining: carriedOver }
      newNodes.forEach((node, i) => {
        recordBlockRuns(editor, node as Node, [stable + i], generation, wordIndex, total, skip)
      })
    }
  })
}

/** The fade delay a DECORATED leaf (`props.leaf`) carries, if any. */
export function freshLeafDelay(leaf: TText): number | null {
  const record = leaf as unknown as Record<string, unknown>
  if (typeof record[CHAT_FRESH_MARK] !== 'number') return null
  const delay = record[CHAT_FRESH_DELAY_MARK]
  return typeof delay === 'number' ? delay : 0
}

// First paint per run, keyed by `bornAt`: one chunk's words resolve in a single
// commit that may straddle a frame, and a shared origin keeps their cascade.
const firstPaintedAt = new Map<number, number>()
// Only has to stop unbounded growth across a long session.
const MAX_TRACKED_PAINTS = 512

function paintOriginFor(bornAt: number, now: number): number {
  const seen = firstPaintedAt.get(bornAt)
  if (seen !== undefined) return seen
  if (firstPaintedAt.size >= MAX_TRACKED_PAINTS) {
    // Insertion-ordered, so the oldest half goes first.
    let drop = MAX_TRACKED_PAINTS / 2
    for (const key of firstPaintedAt.keys()) {
      if (drop-- <= 0) break
      firstPaintedAt.delete(key)
    }
  }
  firstPaintedAt.set(bornAt, now)
  return now
}

/**
 * The `animation-delay` to mount with: the word's wait minus time already on
 * screen, so a remount resumes rather than restarts (negative = started that
 * long ago). Call once per mount; clamped at the fade's length.
 */
export function resumedFadeDelay(leaf: TText): number | null {
  const base = freshLeafDelay(leaf)
  if (base === null) return null
  const bornAt = (leaf as unknown as Record<string, unknown>)[CHAT_FRESH_BORN_MARK]
  if (typeof bornAt !== 'number') return base
  const now = typeof performance === 'object' ? performance.now() : Date.now()
  // Floored to whole frames so a first-frame mount gets its nominal delay exactly.
  const elapsed = Math.max(0, Math.floor((now - paintOriginFor(bornAt, now)) / FRAME_MS) * FRAME_MS)
  return Math.max(base - elapsed, -FADE_DURATION_MS)
}
