import type { DecoratedRange } from '@platejs/slate'
import type { NodeEntry, Path, TText, Value } from 'platejs'
import type { PlateEditor } from 'platejs/react'

type Node = { text?: string; children?: Node[] } & Record<string, unknown>

/** The mark `chat-fresh-text-plugin.tsx` renders as a fade-in. Never written
 *  into the document: it is produced by `freshDecorations` below, as a Slate
 *  DECORATION over text a full markdown parse already produced. See that
 *  function for why the split lives in the render layer and not the document. */
export const CHAT_FRESH_MARK = 'chatFresh'
/** This leaf's `animation-delay`, in ms — see `staggerDelay` below. */
export const CHAT_FRESH_DELAY_MARK = 'chatFreshDelay'

// Keys equality must look past: `NodeIdPlugin` (registered in
// chat-composer-plugins.ts) stamps a fresh random id onto every block it
// normalizes, so two parses of identical markdown never carry the same one —
// it is Plate's own "not content" prop (`isMetadataProp` flags exactly this
// key).
const IGNORED_KEYS = new Set(['id'])

// A whole delta can be a full sentence (Claude's hook) or a few words
// (Codex's stream) — the transport's chunking is not ours to change (see the
// 2026-08-28 investigation). What's ours is staging the REVEAL of whatever
// arrived: each word gets a bit more delay than the last, so one large
// chunk cascades in like a run of smaller ones would have.
//
// The step SCALES DOWN for a long chunk rather than the delay CAPPING —
// capping was the first version of this, and it was wrong: every word past
// the cap shared the exact same delay, so a long sentence visibly split into
// "a handful of words stagger nicely" followed by "the rest of the sentence
// fades in as one abrupt batch". Every word gets a distinct delay here,
// however long the chunk, and the whole cascade still finishes within
// MAX_STAGGER_MS either way.
const WORD_STAGGER_MS = 30
const MAX_STAGGER_MS = 320
export function staggerDelay(index: number, total: number): number {
  const step = total <= 1 ? 0 : Math.min(WORD_STAGGER_MS, MAX_STAGGER_MS / (total - 1))
  return index * step
}

// A hidden word already occupies its final layout position the instant its
// chunk lands, so the transcript's scroll-follow (a separate, continuously
// running loop) starts converging on the new bottom immediately —
// concurrently with, not after, the word cascade below. This flat head start
// on every word in a chunk (added on top of staggerDelay, not inside it —
// that function's own tests assert its bare per-word values) lets the scroll
// get underway before text starts materializing, so a chunk reads as
// "settles into view, then fills in" rather than everything at once.
const SCROLL_LEAD_MS = 150

// Above this many words in ONE insertion, per-word splitting is pure cost
// with nothing to show for it: staggerDelay's own step already collapses
// toward sub-millisecond well before this many words share MAX_STAGGER_MS,
// so a huge jump's word-by-word stagger is already visually indistinguishable
// from one fade. The whole chunk still fades in as one animated unit rather
// than popping in unanimated.
const WORD_SPLIT_CAP = 80

/** Splits text into whitespace-preserving chunks — concatenating the result
 *  reconstructs the original string exactly, including leading/repeated
 *  whitespace, unlike a plain `.split(' ')`. */
export function splitIntoWords(text: string): string[] {
  return text.match(/\s*\S+\s*/g) ?? [text]
}

/**
 * One insertion's worth of just-arrived text, recorded so `freshDecorations`
 * can fade it in — the streaming equivalent of a selection: a range over text
 * the document already holds, never a change to that text.
 */
interface FreshRun {
  /** Which insertion this is. Distinct per call so two runs that end up
   *  adjacent still animate on their own timing rather than as one. */
  generation: number
  /** The text leaf this run lives in, and the character range within it. */
  path: Path
  start: number
  end: number
  /** This run's first word's index within its whole generation, and that
   *  generation's total — one insertion can span several leaves (a new
   *  heading and its body), and the cascade has to read as one. */
  wordOffset: number
  totalWords: number
}

// PERFORMANCE, measured (Chrome 152, a 510-word reply in 64 flushes): the
// per-word fade used to be one Slate LEAF per word, written into the
// document. That made a chunk cost one `insert_node` operation per word, and
// each settling word another `unset_node` — and every Slate operation runs
// the whole plugin stack's `apply`/`normalizeNode` overrides. `ListPlugin`'s
// alone (@platejs/list, registered for agent replies that use bullets) makes
// an operation ~1.4ms, so an ordinary 8-word chunk cost ~11ms of the 16.7ms
// frame: the transcript rendered at ~24fps while streaming, and the fade and
// scroll-follow — both rAF-driven — starved together.
//
// Decorations are the fix, and they are the RIGHT primitive rather than a
// trick: a decoration is a range over existing text that Slate splits into
// leaves for RENDERING only. The document stays exactly what the markdown
// parse produced (one leaf), a chunk costs ONE `insert_text` operation
// regardless of word count, and settling costs none at all. Same DOM, same
// animation, 21x less main-thread work (688ms -> 33ms over that reply).
const freshRuns = new WeakMap<PlateEditor, FreshRun[]>()
const settledGenerations = new WeakMap<PlateEditor, Set<number>>()
const freshGenerations = new WeakMap<PlateEditor, number>()

// A backstop, not the ordinary retirement path — `settleFreshGeneration`
// (fired by the real `animationend`) is. This bounds the run list for the
// cases where that event never arrives at all: `prefers-reduced-motion`
// zeroes the animation (transcript.css), and a chat kept mounted but hidden
// runs no animations to end. At the batcher's ceiling of one flush per frame
// this is ~1s of history against a cascade that finishes in 470ms, so a run
// is only ever dropped here long after it has visually settled.
const MAX_LIVE_GENERATIONS = 60

function nextFreshGeneration(editor: PlateEditor): number {
  const next = (freshGenerations.get(editor) ?? 0) + 1
  freshGenerations.set(editor, next)
  return next
}

function pathEquals(a: Path, b: Path): boolean {
  return a.length === b.length && a.every((step, i) => step === b[i])
}

function recordRun(editor: PlateEditor, run: FreshRun): void {
  const runs = freshRuns.get(editor) ?? []
  runs.push(run)
  freshRuns.set(editor, runs)
}

/** Drops runs that can no longer be rendered: already settled, aged out of
 *  the generation window, or living in a block this patch is about to
 *  rewrite (their character offsets would no longer mean anything). */
function pruneRuns(editor: PlateEditor, invalidFromBlock: number): void {
  const runs = freshRuns.get(editor)
  if (!runs?.length) return
  const settled = settledGenerations.get(editor)
  const floor = (freshGenerations.get(editor) ?? 0) - MAX_LIVE_GENERATIONS
  const kept = runs.filter(
    (run) =>
      !settled?.has(run.generation) &&
      run.generation > floor &&
      (run.path[0] ?? 0) < invalidFromBlock,
  )
  freshRuns.set(editor, kept)
  if (settled && kept.length === 0) settled.clear()
}

/**
 * Retires one finished fade. Called from the real `animationend` — never a
 * timer — and costs no editor operation at all: a settled generation is
 * simply one `freshDecorations` stops emitting.
 *
 * Nothing has to re-render for this to be correct. The animation ends at
 * full opacity and `fill-mode: both` holds it there, so a decoration that
 * outlives its own animation until the next streamed chunk re-renders the
 * block looks exactly like the plain text it will become.
 */
export function settleFreshGeneration(editor: PlateEditor, generation: number): void {
  const settled = settledGenerations.get(editor) ?? new Set<number>()
  settled.add(generation)
  settledGenerations.set(editor, settled)
}

/**
 * The fade, as Slate ranges over text the document already holds.
 *
 * Returns one range per word — each carrying its own `CHAT_FRESH_DELAY_MARK`
 * so the chunk cascades in — for whichever recorded runs live in `entry`'s
 * leaf. Slate splits the leaf along these ranges when it renders it, which is
 * what produces the per-word `<span>`s the CSS animates, without the document
 * ever holding a leaf per word.
 */
export function freshDecorations(editor: PlateEditor, [node, path]: NodeEntry): DecoratedRange[] {
  const text = (node as Node).text
  if (typeof text !== 'string') return []
  const runs = freshRuns.get(editor)
  if (!runs?.length) return []
  const settled = settledGenerations.get(editor)

  const ranges: DecoratedRange[] = []
  for (const run of runs) {
    if (!pathEquals(run.path, path)) continue
    if (settled?.has(run.generation)) continue
    // A reparse can reshape the leaf this run was recorded against (an
    // inline mark opening mid-word splits it). Its offsets then name text
    // that is no longer there, so the run is dropped rather than guessed at.
    if (run.end > text.length || run.start >= run.end) continue

    const body = text.slice(run.start, run.end)
    // See WORD_SPLIT_CAP: past this many words the stagger step is already
    // imperceptible, and one range beats hundreds of DOM spans.
    if (run.totalWords > WORD_SPLIT_CAP) {
      ranges.push({
        anchor: { path, offset: run.start },
        focus: { path, offset: run.end },
        [CHAT_FRESH_MARK]: run.generation,
        [CHAT_FRESH_DELAY_MARK]: SCROLL_LEAD_MS,
      } as unknown as DecoratedRange)
      continue
    }
    let offset = run.start
    splitIntoWords(body).forEach((word, i) => {
      const next = offset + word.length
      ranges.push({
        anchor: { path, offset },
        focus: { path, offset: next },
        [CHAT_FRESH_MARK]: run.generation,
        [CHAT_FRESH_DELAY_MARK]: SCROLL_LEAD_MS + staggerDelay(run.wordOffset + i, run.totalWords),
      } as unknown as DecoratedRange)
      offset = next
    })
  }
  return ranges
}

// How many of `editor.children`'s LEADING blocks are already confirmed to
// match a fresh reparse, as of the last call. `applyStreamedValue` never
// touches anything before its own `stable` boundary once computed — so a
// block confirmed stable stays stable for the rest of this editor's life,
// and re-comparing it on every later token is exactly the "touch everything,
// not just what changed" cost this module exists to avoid. Left unchecked
// this turns one streamed message into O(length²) work: by message end,
// EVERY already-settled paragraph gets walked again on EVERY remaining token.
const knownStablePrefix = new WeakMap<PlateEditor, number>()

function nodesEqual(a: unknown, b: unknown): boolean {
  if (a === b) return true
  if (typeof a !== 'object' || a === null || typeof b !== 'object' || b === null) return false
  if (Array.isArray(a) || Array.isArray(b)) {
    if (!Array.isArray(a) || !Array.isArray(b) || a.length !== b.length) return false
    return a.every((item, i) => nodesEqual(item, b[i]))
  }
  const ao = a as Record<string, unknown>
  const bo = b as Record<string, unknown>
  const keysA = Object.keys(ao).filter((k) => !IGNORED_KEYS.has(k))
  const keysB = Object.keys(bo).filter((k) => !IGNORED_KEYS.has(k))
  if (keysA.length !== keysB.length) return false
  return keysA.every((k) => k in bo && nodesEqual(ao[k], bo[k]))
}

// Excludes `text` too: this also gates the leaf branch below, where the whole
// point is that `text` is ALLOWED to differ — it is the marks (bold, code,
// a link's url, ...) that must match for a text change to be a pure append.
function ownPropsEqual(a: Node, b: Node): boolean {
  const keysA = Object.keys(a).filter(
    (k) => !IGNORED_KEYS.has(k) && k !== 'children' && k !== 'text',
  )
  const keysB = Object.keys(b).filter(
    (k) => !IGNORED_KEYS.has(k) && k !== 'children' && k !== 'text',
  )
  if (keysA.length !== keysB.length) return false
  return keysA.every((k) => nodesEqual(a[k], b[k]))
}

/** The rightmost text leaf of a node — where a trailing append always lands. */
function lastLeaf(node: Node): Node {
  if (typeof node.text === 'string') return node
  const children = node.children ?? []
  return lastLeaf(children[children.length - 1] ?? {})
}

/** How many word-chunks a node's text spans — needed BEFORE recording runs,
 *  since every word in one insertion shares a stagger step sized off the
 *  total (see `staggerDelay`). */
function countWords(node: Node): number {
  if (typeof node.text === 'string') return splitIntoWords(node.text).length
  if (!node.children) return 0
  return node.children.reduce((sum, child) => sum + countWords(child), 0)
}

/** Records a fade over every text leaf of a newly-inserted block, threading
 *  one continuous word index through all of them so a heading and the body
 *  under it cascade as one run rather than restarting. */
function recordBlockRuns(
  editor: PlateEditor,
  node: Node,
  path: Path,
  generation: number,
  wordIndex: { current: number },
  total: number,
): void {
  if (typeof node.text === 'string') {
    if (node.text.length === 0) return
    recordRun(editor, {
      generation,
      path,
      start: 0,
      end: node.text.length,
      wordOffset: wordIndex.current,
      totalWords: total,
    })
    wordIndex.current += splitIntoWords(node.text).length
    return
  }
  node.children?.forEach((child, i) => {
    recordBlockRuns(editor, child, [...path, i], generation, wordIndex, total)
  })
}

/** How many leading top-level blocks `prev` and `next` already agree on. */
export function stableBlockCount(prev: Value, next: Value): number {
  const max = Math.min(prev.length, next.length)
  let i = 0
  while (i < max && nodesEqual(prev[i], next[i])) i++
  return i
}

export interface TextDivergence {
  /** How many of `prev`'s trailing leaf's characters are still correct —
   *  left untouched by the caller, never re-marked or re-animated. */
  keep: number
  /** What replaces everything in that leaf after `keep` characters — freshly
   *  marked to fade in. Empty when `next`'s trailing text is a strict
   *  prefix of `prev`'s (reconciliation made it shorter, nothing to add). */
  replacement: string
}

/**
 * Whether `next`'s trailing text leaf is a pure append to `prev`'s (a
 * paragraph or code line still being typed) IS the case where `keep` comes
 * back equal to the whole of `prev`'s own trailing text — this function
 * covers that and the shape a pure append can't express: `next`'s trailing
 * leaf sharing only a PARTIAL prefix with `prev`'s, exactly what
 * turn/message.go's closeAssistantTurn produces when the terminating hook's
 * own final text disagrees with what streamed and reconciliation replaces
 * it outright rather than merely extending it.
 *
 * Returns null for a STRUCTURAL reason only: a mark changed, a non-trailing
 * leaf changed, or the child count or block type changed — anything that
 * isn't "prev and next's trailing leaves share a common prefix, however
 * short". Provider-agnostic on purpose: reconciliation is not a Codex-only
 * behavior (see closeAssistantTurn's own doc comment), so this generalizes
 * for whichever provider triggers it, not a provider-specific carve-out.
 */
export function trailingTextDivergence(prev: Node, next: Node): TextDivergence | null {
  const prevIsText = typeof prev.text === 'string'
  const nextIsText = typeof next.text === 'string'
  if (prevIsText !== nextIsText) return null
  if (prevIsText && nextIsText) {
    if (!ownPropsEqual(prev, next)) return null
    const prevText = prev.text as string
    const nextText = next.text as string
    const max = Math.min(prevText.length, nextText.length)
    let keep = 0
    while (keep < max && prevText[keep] === nextText[keep]) keep++
    return { keep, replacement: nextText.slice(keep) }
  }
  if (!ownPropsEqual(prev, next)) return null
  const prevChildren = prev.children ?? []
  const nextChildren = next.children ?? []
  if (prevChildren.length !== nextChildren.length) return null
  for (let i = 0; i < prevChildren.length; i++) {
    if (nodesEqual(prevChildren[i], nextChildren[i])) continue
    // A divergence anywhere but the last child is structural, not a stream
    // growing (or reconciling) in place — a later reparse changed something
    // behind the tail.
    if (i !== prevChildren.length - 1) return null
    return trailingTextDivergence(prevChildren[i], nextChildren[i])
  }
  // Every child matched exactly — prev's own trailing text is entirely kept.
  return { keep: (lastLeaf(prev).text as string).length, replacement: '' }
}

/**
 * Applies `next` to `editor` by touching only the blocks that actually
 * differ from what it already holds.
 *
 * This exists because `editor.tf.setValue` (and recreating the editor on
 * every token, which is what this replaces) both remove every top-level node
 * and reinsert the whole document — see replaceNodes.ts. Neither one "only
 * touches what changed": every already-settled paragraph gets torn down and
 * rebuilt alongside the one still growing, which is the per-token cost the
 * 2026-08-24 performance plan measured. Leading blocks that compare equal
 * (stableBlockCount) are never removed or reinserted at all, and the common
 * case — a paragraph or code line growing token by token — becomes a single
 * `insert_text` operation.
 *
 * What lands is recorded as a fresh RUN (see `freshRuns`) rather than written
 * into the document as marked-up leaves, so the fade costs no operations of
 * its own. The document this leaves behind is therefore exactly what a plain
 * markdown parse of the same text produces, modulo `NodeIdPlugin`'s ids —
 * which is what lets the next call compare against it directly.
 */
export function applyStreamedValue(editor: PlateEditor, next: Value): void {
  const prev = editor.children as Value
  const startAt = Math.min(knownStablePrefix.get(editor) ?? 0, prev.length, next.length)
  const stable = startAt + stableBlockCount(prev.slice(startAt), next.slice(startAt))
  knownStablePrefix.set(editor, stable)
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
          // A pure append (staleLength === 0) needs no delete — the common
          // case. Anything reached here with staleLength > 0 is what a pure
          // append couldn't express: the terminating hook's own text
          // reconciled part of the tail to something DIFFERENT, not just
          // longer (closeAssistantTurn, provider-agnostic). Removing only the
          // diverging suffix — never the shared prefix before it — is what
          // keeps that prefix's fade state untouched instead of re-triggering
          // the whole block's cascade.
          if (staleLength > 0) {
            // Every recorded offset in this block is measured against text
            // that is about to change length, so those runs go with it.
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
            // Re-read after the delete above: deleting a range shifts every
            // point after it, so the pre-delete `endPoint` no longer names
            // the block's end once something was removed.
            const insertAt = staleLength > 0 ? editor.api.end([stable]) : endPoint
            if (insertAt) {
              // ONE operation, whatever the chunk's word count — the fade's
              // per-word split is a decoration over this text, not a leaf per
              // word in the document. See `freshRuns`.
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
    }
    pruneRuns(editor, stable)
    let removing = prev.length - stable
    while (removing-- > 0) editor.tf.removeNodes({ at: [stable] })
    if (stable < next.length) {
      const newNodes = next.slice(stable)
      const generation = nextFreshGeneration(editor)
      const wordIndex = { current: 0 }
      const total = newNodes.reduce((sum, node) => sum + countWords(node as Node), 0)
      // One `insert_node` per BLOCK — a block's whole subtree rides along
      // inside that single operation, so this stays O(blocks), never O(words).
      editor.tf.insertNodes(newNodes as Value, { at: [stable] })
      newNodes.forEach((node, i) => {
        recordBlockRuns(editor, node as Node, [stable + i], generation, wordIndex, total)
      })
    }
  })
}

/** The fade marks a decorated leaf carries, if any — read from the DECORATED
 *  leaf (`props.leaf`), never the underlying text node, which never holds
 *  them. */
export function freshLeafDelay(leaf: TText): number | null {
  const record = leaf as unknown as Record<string, unknown>
  if (typeof record[CHAT_FRESH_MARK] !== 'number') return null
  const delay = record[CHAT_FRESH_DELAY_MARK]
  return typeof delay === 'number' ? delay : 0
}
