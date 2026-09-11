import type { DecoratedRange } from '@platejs/slate'
import type { NodeEntry, Path, TText, Value } from 'platejs'
import type { PlateEditor } from 'platejs/react'

type Node = { text?: string; children?: Node[] } & Record<string, unknown>

/** The mark `chat-fresh-text-plugin.tsx` renders as a fade-in. Never written
 *  into the document: it is produced by `freshDecorations` below, as a Slate
 *  DECORATION over text a full markdown parse already produced. See that
 *  function for why the split lives in the render layer and not the document. */
export const CHAT_FRESH_MARK = 'chatFresh'
/** This leaf's `animation-delay`, in ms — see `staggerDelay` below.
 *
 *  CONSTANT for the life of the run that produced it. It is tempting to bake
 *  "how much of the wait is left" into this instead, since that is what a
 *  remounted span needs; doing so makes the value change on every recompute,
 *  and a decoration whose content changes is a decoration `isTextDecorationsEqual`
 *  can never match — so every block holding a fading word re-renders on every
 *  delta, and a word already on screen has the `animation-delay` of its RUNNING
 *  animation rewritten underneath it, which jumps that animation's current time
 *  and repaints settled text. That is the "text renders again when the sentence
 *  finishes" report, and the `chat-fresh-text-plugin` test that pins a fading
 *  word's delay across a neighbour's settle catches it. The elapsed part is
 *  applied ONCE, at mount, by the leaf — see `resumedFadeDelay`. */
export const CHAT_FRESH_DELAY_MARK = 'chatFreshDelay'
/** When the run this leaf belongs to landed (`performance.now()`). Constant,
 *  and the only time-derived thing a decoration carries — the leaf turns it
 *  into "how much of the wait is left" once, at mount. */
export const CHAT_FRESH_BORN_MARK = 'chatFreshBornAt'
/** A finished fade still holding its place in the leaf split so the words
 *  after it keep their identity — see `pruneRuns`. Deliberately matches NO
 *  plugin: it splits the leaf (which is its whole job) and renders as the
 *  plain text it now is, with no animation to restart. */
export const CHAT_FRESH_HELD = 'chatFreshHeld'
/** This word's own index within its whole generation's cascade, and that
 *  generation's total word count — carried ONLY on a per-word split (never
 *  on a capped run's single, unsplit span; see WORD_SPLIT_CAP) so the leaf's
 *  `animationend` can report exactly one word done via `settleFreshWord`,
 *  instead of the whole run's shared `generation` — which would retire every
 *  OTHER word sharing it the instant the FIRST one finishes, since
 *  `staggerDelay(0, n)` is always 0 and so always finishes first. */
export const CHAT_FRESH_WORD_INDEX_MARK = 'chatFreshWordIndex'
export const CHAT_FRESH_WORD_TOTAL_MARK = 'chatFreshWordTotal'

// Keys equality must look past, because a PLUGIN derives them rather than the
// markdown carrying them — so the document and a fresh parse of the very same
// text disagree about them forever, and a diff that respected them would
// consider every block permanently changed.
//
// - `id`: `NodeIdPlugin` (registered in chat-composer-plugins.ts) stamps a
//   fresh random one onto every block it normalizes, so two parses of
//   identical markdown never match. It is Plate's own "not content" prop
//   (`isMetadataProp` flags exactly this key).
// - `listStart`: `@platejs/list`'s `normalizeListStart` renumbers ordered
//   items from their position and DELETES the prop from the first item, whose
//   `1` is implicit — while the markdown parse always emits it. Measured, in
//   Chrome, on a streamed 10-item numbered list: this one prop on this one
//   block dropped `stableBlockCount` to 0 of 10, so every flush tore the whole
//   list down and reinserted it, and every reinsertion re-entered that same
//   renumbering pass — 121 Slate operations per flush on a 24-item list, a
//   42ms median frame and a 571ms freeze. Both are derived, both are the
//   plugin's to maintain, and neither is what the agent actually said.
const IGNORED_KEYS = new Set(['id', 'listStart'])

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

// How long `chat-token-fade` itself runs — keep in step with transcript.css.
const FADE_DURATION_MS = 260

// One frame, the resolution `resumedFadeDelay` measures a run's age in.
const FRAME_MS = 16

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
  /** When this text landed, as a `performance.now()` reading — stamped by
   *  `recordRun`, never by a caller.
   *
   *  THE FADE IS PLAYED AGAINST THIS CLOCK, NOT AGAINST THE SPAN'S OWN LIFE.
   *  A `.chat-fresh-text` span holds `animation-fill-mode: both` over a
   *  keyframe that starts at `opacity: 0`, so a span that is unmounted and
   *  remounted starts its fade AGAIN from invisible — and slate-react
   *  remounts these constantly, because it keys each rendered leaf by its
   *  positional index into a decoration split this module rebuilds on every
   *  delta (measured live on one streamed 16-item list: 1918 leaf mounts for
   *  887 animation starts, 61 of them killed before `animationend`). Without a
   *  birth time there is nothing to measure that against: every remount looks
   *  like brand-new text, so under a fast stream the restarts outrun the
   *  fades and already-arrived words sit at the animation's invisible start
   *  state for as long as the stream keeps going — the list whose bullets are
   *  on screen with nothing under them. */
  bornAt: number
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
 * Drops runs that can no longer be rendered — but never one whose BOUNDARY a
 * still-fading neighbour depends on.
 *
 * REGRESSION this shape exists to prevent, root-caused in slate's own source:
 * `slate-react` keys each rendered leaf `${textKey}-${i}`, where `i` is the
 * positional index into the split `Text.decorations` rebuilds from scratch
 * every render. Dropping ONE settled range therefore shifts the index of
 * every leaf after it in the same text node — React sees new keys, unmounts
 * those spans, mounts fresh ones, and a fresh DOM node starts its CSS
 * animation from zero. Mid-stream that happened continuously, so words never
 * got an uninterrupted stretch of real time to finish fading and sat at the
 * animation's invisible start state until the stream stopped.
 *
 * So a settled run keeps holding its boundary (rendered inert — see
 * `freshDecorations`) for as long as ANY run in the same text node is still
 * fading, and a line's runs are only ever released together, once nothing
 * there is animating and the reindex can touch only inert leaves.
 */
function pruneRuns(editor: PlateEditor, invalidFromBlock: number): void {
  const runs = freshRuns.get(editor)
  if (!runs?.length) return
  const settled = settledGenerations.get(editor)
  const floor = (freshGenerations.get(editor) ?? 0) - MAX_LIVE_GENERATIONS
  const live = (run: FreshRun) => !settled?.has(run.generation) && run.generation > floor
  // The text nodes that still have something fading in them. Everything else
  // is free to go: a line with no live run left can collapse its whole split
  // at once without disturbing an animation, because there is none to disturb.
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

// One cleanup pass per frame, however many words finished in it — see
// `settleFreshGeneration`. Coalesced the same way the delta batcher coalesces
// store writes (streaming-message-batcher.ts), because a chunk's words all
// finish within a frame or two of each other and each one asking for its own
// pass would mean a pass per word.
const pendingCleanup = new WeakMap<PlateEditor, number>()

function scheduleFadeCleanup(editor: PlateEditor): void {
  if (typeof requestAnimationFrame !== 'function') return
  if (pendingCleanup.has(editor)) return
  pendingCleanup.set(
    editor,
    requestAnimationFrame(() => {
      pendingCleanup.delete(editor)
      // Absent until an editor is actually mounted in React — a headless one
      // (the markdown codec's, or a test's) has no rendering to invalidate.
      editor.api.redecorate?.()
    }),
  )
}

/**
 * Retires one finished fade. Called from the real `animationend` — never a
 * timer — and costs no editor operation at all: a settled generation is
 * simply one `freshDecorations` stops emitting as animated.
 *
 * REGRESSION this schedules a cleanup pass for: settling is bookkeeping, and
 * a block only recomputes its decorations when something re-renders it. While
 * a LIST streams, only the last item is ever re-rendered — so every finished
 * item kept its animated spans in the DOM for the rest of the turn, each one
 * an element still carrying an `animation` declaration `fill-mode: both`
 * keeps alive. Measured on a 30-item list: 314 of them by the end, released
 * only when the stream stopped. The design this replaced never had the
 * problem because settling unset the marks, which merged the leaves back to
 * plain text and removed the spans outright.
 *
 * `redecorate` is the whole pass, and it is cheaper than it sounds: it bumps
 * the decoration version, so decorations are recomputed (a fast miss for
 * every leaf holding no run) but `isTextDecorationsEqual` still gates the
 * re-render, and only blocks whose decorations actually changed re-render.
 */
export function settleFreshGeneration(editor: PlateEditor, generation: number): void {
  const settled = settledGenerations.get(editor) ?? new Set<number>()
  if (settled.has(generation)) return
  settled.add(generation)
  settledGenerations.set(editor, settled)
  scheduleFadeCleanup(editor)
}

// Per-generation set of word indices that have reported their own
// `animationend` — see `settleFreshWord`.
const settledWordCounts = new WeakMap<PlateEditor, Map<number, Set<number>>>()

/**
 * One word of a per-word cascade finished its own fade — called from
 * `chat-fresh-text-plugin.tsx` only when the leaf carries
 * `CHAT_FRESH_WORD_INDEX_MARK` (a per-word split; see `freshDecorations`).
 *
 * Unlike `settleFreshGeneration` (called directly only for a capped run's
 * single, unsplit span — see `WORD_SPLIT_CAP`), this waits for EVERY word
 * sharing `generation` to report before retiring it. Settling on the first
 * word's `animationend` alone — which the old code did, because every word
 * shared one `generation` — is exactly the bug this exists to fix:
 * `staggerDelay(0, n)` is always 0, so the first word always finishes
 * first, and settling then made `freshDecorations` render every OTHER word
 * in the same chunk as instantly inert (`CHAT_FRESH_HELD`) before its own
 * staggered delay had even elapsed — the cascade never played past word one.
 */
export function settleFreshWord(
  editor: PlateEditor,
  generation: number,
  wordIndex: number,
  totalWords: number,
): void {
  const byGeneration = settledWordCounts.get(editor) ?? new Map<number, Set<number>>()
  const words = byGeneration.get(generation) ?? new Set<number>()
  // A Set, not a counter: pruneRuns holds a settled leaf's SPLIT stable
  // rather than dropping it while a neighbour still fades, which can replay
  // the same word's `animationend` on remount. Idempotent add is what makes
  // a repeat call harmless instead of over-counting toward `totalWords`.
  words.add(wordIndex)
  byGeneration.set(generation, words)
  settledWordCounts.set(editor, byGeneration)
  if (words.size >= totalWords) settleFreshGeneration(editor, generation)
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

  const floor = (freshGenerations.get(editor) ?? 0) - MAX_LIVE_GENERATIONS

  const ranges: DecoratedRange[] = []
  for (const run of runs) {
    if (!pathEquals(run.path, path)) continue
    // A reparse can reshape the leaf this run was recorded against (an
    // inline mark opening mid-word splits it). Its offsets then name text
    // that is no longer there, so the run is dropped rather than guessed at.
    if (run.end > text.length || run.start >= run.end) continue
    // Still held only to keep the split stable for a fading neighbour (see
    // `pruneRuns`). It carries no mark any plugin renders, so it is plain
    // text that merely happens to be its own leaf — and, crucially, no
    // animation to be restarted if React does remount it.
    //
    // Deliberately NOT time-based. Retiring a run on a clock here reads as a
    // second render of text that was already on screen: the leaf stops being
    // the fade plugin's element and becomes a plain one, so React tears the
    // span (and its compositor layer) down and rebuilds the text mid-sentence.
    // The stranded-invisible case that motivated a clock is handled where it
    // belongs instead — at mount, by `resumedFadeDelay`, which can never leave
    // a word waiting longer than its own window however often it remounts.
    const held = settled?.has(run.generation) || run.generation <= floor
    // `wordIndex` is present only on a per-word split (never the capped
    // branch below) — see CHAT_FRESH_WORD_INDEX_MARK's own doc for why that
    // distinction matters to how this word's OWN animationend settles.
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

    // See WORD_SPLIT_CAP: past this many words the stagger step is already
    // imperceptible, and one range beats hundreds of DOM spans. Genuinely one
    // fade, not a cascade, so it settles directly (no word index) same as
    // ever — there is no first-word-finishes-early bug when there's only one.
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

/** One leaf whose marks and/or text differ between `prev` and `next`, at the
 *  path it lives at in BOTH trees (identical, since `leafDivergences` only
 *  returns any when the two share the same shape everywhere else). */
interface LeafDiff {
  path: Path
  prev: Node
  next: Node
}

/**
 * Collects every LEAF where `prev` and `next` differ, PROVIDED the two trees
 * share the exact same shape everywhere else (same element props at every
 * non-leaf level, same children count at every level) — pushing to `out` and
 * returning `true`. Returns `false` for a genuinely STRUCTURAL difference (an
 * element's own props changed, or a children array grew/shrank/reordered)
 * without collecting anything: the caller's cue that this pair cannot be
 * patched leaf-by-leaf and needs the full block replaced instead.
 *
 * This is what tells "a markdown span's closing syntax just landed" (a plain
 * leaf becoming a `bold`/`code` one — same leaf, same position, only its own
 * marks and text changed) apart from "the paragraph's shape itself changed"
 * (a code span splitting one leaf into three where there was one, a list
 * item gaining a sibling). Only the first case can be patched leaf-by-leaf
 * without touching any element's own identity.
 */
function leafDivergences(prev: Node, next: Node, path: Path, out: LeafDiff[]): boolean {
  const prevIsText = typeof prev.text === 'string'
  const nextIsText = typeof next.text === 'string'
  if (prevIsText !== nextIsText) return false
  if (prevIsText && nextIsText) {
    if (prev.text !== next.text || !ownPropsEqual(prev, next)) out.push({ path, prev, next })
    return true
  }
  if (!ownPropsEqual(prev, next)) return false
  const prevChildren = prev.children ?? []
  const nextChildren = next.children ?? []
  if (prevChildren.length !== nextChildren.length) return false
  for (let i = 0; i < prevChildren.length; i++) {
    if (!leafDivergences(prevChildren[i]!, nextChildren[i]!, [...path, i], out)) return false
  }
  return true
}

/** This leaf's own mark keys — everything but `text`/`children` and the
 *  derived props `ownPropsEqual` already ignores. */
function markKeys(node: Node): string[] {
  return Object.keys(node).filter((k) => !IGNORED_KEYS.has(k) && k !== 'children' && k !== 'text')
}

/**
 * Applies one leaf's worth of `LeafDiff`s IN PLACE: `setNodes`/`unsetNodes`
 * for whatever marks changed, `delete`+`insertText` for whatever text
 * changed — never `removeNodes`/`insertNodes` on the leaf OR any ancestor.
 * That is the whole point: every element above these leaves (the block
 * itself, any wrapper) keeps the exact node reference and `id` it already
 * had, so nothing about it is a fresh insert to `NodeIdPlugin`, and nothing
 * about it forces the block's own render identity to change.
 */
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
      // The whole leaf, not just a suffix: unlike a pure append, there is no
      // meaningful "already-visible prefix" here — the leaf's old text is
      // gone the instant its marks changed (that IS the edit), so all of its
      // new text is genuinely new to the screen.
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

/** The rightmost text leaf of a node — where a trailing append always lands. */
function lastLeaf(node: Node): Node {
  if (typeof node.text === 'string') return node
  const children = node.children ?? []
  return lastLeaf(children[children.length - 1] ?? {})
}

/** Records a fade over every text leaf of a newly-inserted block, threading
 *  one continuous word index through all of them so a heading and the body
 *  under it cascade as one run rather than restarting.
 *
 *  `skip` is characters of the block's FLATTENED text to treat as already
 *  seen — not fresh, not re-animated — even though this whole block is being
 *  torn down and reinserted. See `replaceTrailingBlock`: a mark completing
 *  mid-paragraph forces a full block replace, and without this every already
 *  -settled word in that paragraph would flash and re-fade along with the one
 *  word whose markup actually just resolved. Mutated as leaves consume it, so
 *  callers share one counter across the whole subtree. */
function recordBlockRuns(
  editor: PlateEditor,
  node: Node,
  path: Path,
  generation: number,
  wordIndex: { current: number },
  total: number,
  // `take` bounds how much of the subtree past `remaining` still counts as
  // fresh — left at Infinity (its default) for a whole-new-block insert,
  // where everything past the skip genuinely IS new all the way to the
  // block's own end. The mid-paragraph mark-completing caller narrows it to
  // the exact freshSuffix length: without a stop, this would keep marking
  // fresh past that suffix too, all the way to the block's real end, which
  // re-flashes a trailing run of text neither edit touched. See
  // commonSuffixLength's own doc for why that text needs excluding at all.
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

/** Flattened text of a node's whole subtree, leaf order — the same shape
 *  `countWords` walks, but concatenated rather than counted. Used only to
 *  find how much of a replaced block's text was already on screen; never
 *  read to decide what Slate operation runs. */
function flattenText(node: Node): string {
  if (typeof node.text === 'string') return node.text
  return (node.children ?? []).map(flattenText).join('')
}

/** How many of `a` and `b`'s leading characters agree, ignoring marks
 *  entirely — the flattened-text analogue of `stableBlockCount`, one level
 *  down. Marks are exactly what a completing markdown span changes, so a
 *  mark-aware comparison here would defeat the point: this exists to tell
 *  "already visible" apart from "genuinely new" when marks are precisely
 *  what a full block replace can no longer preserve leaf-for-leaf. */
function commonPrefixLength(a: string, b: string): number {
  const max = Math.min(a.length, b.length)
  let i = 0
  while (i < max && a[i] === b[i]) i++
  return i
}

/** How many of `a` and `b`'s TRAILING characters agree — the suffix twin of
 *  `commonPrefixLength`, needed together with it. A mark can complete
 *  anywhere in the block, not only at the end, and the prefix alone cannot
 *  tell "an earlier mark shrank the text" apart from "genuinely new text was
 *  appended": everything after a mid-block divergence reads as fresh under
 *  the prefix check alone, even text neither edit ever touched. Comparing
 *  from both ends and keeping only the middle — the classic prefix+suffix
 *  diff trick — is what tells "a code span's closing backtick landed here"
 *  from "the reply kept streaming past here." The caller must clamp the two
 *  against the shorter string's length: an identical string reports its
 *  full length from BOTH ends, and unclamped they overlap. */
function commonSuffixLength(a: string, b: string): number {
  const max = Math.min(a.length, b.length)
  let i = 0
  while (i < max && a[a.length - 1 - i] === b[b.length - 1 - i]) i++
  return i
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
  // NEVER LATCH THE LAST BLOCK. `stable` counts blocks that matched THIS call;
  // the final one matching means only "it has not changed yet", never "it is
  // finished" — it is the block the stream is still writing into. Latching it
  // made `startAt` skip past it on every later call, so it was never compared
  // again and every subsequent edit to it was dropped for the life of the
  // editor.
  //
  // Tables are where this bites, because a table is ONE block for a great many
  // deltas and a partially-arrived line frequently reparses to exactly what
  // the previous delta produced (mid-separator-row `| --- | --- | -` parses to
  // the same two paragraphs as the delta before it). One such tick was enough:
  // reproduced live against a real Codex turn, the table froze on its header
  // row for 6.7s while every body row streamed in unseen, then appeared all at
  // once when the turn ended and the row swapped to `MarkdownMessageStatic` —
  // which reparses from scratch and so never saw the stale prefix. Streamed
  // character by character in a test, the editor diverged at the separator row
  // and never recovered: it finished holding three paragraphs where a fresh
  // parse of the same markdown holds a full table.
  //
  // The cost of not latching it is one `nodesEqual` on one block per delta —
  // the same comparison the trailing fast paths below already have to make.
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
      if (!divergence) {
        // trailingTextDivergence found no text-edit shape at all — the common
        // real cause is a mark completing mid-paragraph (a markdown span like
        // **bold** or `code` resolving once its closing syntax arrives), which
        // changes a leaf's OWN props and so reads as structural, not a growing
        // tail.
        //
        // Patch the affected leaves in place when the block's SHAPE (every
        // element's own props, every children array's length) is otherwise
        // identical — the common case for one mark resolving. Confirmed live
        // (instrumented `applyStreamedValue` across a realistic delta stream)
        // that the block-replace fallback below reassigns this block's
        // NodeIdPlugin `id` on every one of these — once per resolved mark,
        // exactly the moment a bold list-item title or an inline code span
        // closes. A block whose render identity is keyed by that `id` remounts
        // on every such edit, which restarts its `.chat-fresh-text` fade
        // (`animation-fill-mode: both`) from its own zero-opacity start —
        // and a block busy resolving several marks in quick succession (a
        // bold header, then an inline code span moments later) never gets an
        // uninterrupted 260ms to finish fading in, matching a report of list
        // items sitting visibly blank while still actively streaming.
        const leafDiffs: LeafDiff[] = []
        if (leafDivergences(prevBlock, nextBlock, [stable], leafDiffs) && leafDiffs.length > 0) {
          pruneRuns(editor, stable)
          patchLeavesInPlace(editor, leafDiffs)
          return
        }
        // Shape genuinely changed (an element's own props, or a children
        // count, differ) — no leaf-by-leaf patch can express that. Nothing
        // else in the paragraph changed either way, so the fade stays scoped
        // to whatever text is actually new rather than re-fading words that
        // were already fully visible a moment ago. See recordBlockRuns' `skip`.
        pruneRuns(editor, stable)
        editor.tf.removeNodes({ at: [stable] })
        editor.tf.insertNodes([nextBlock] as Value, { at: [stable] })
        const prevText = flattenText(prevBlock)
        const nextText = flattenText(nextBlock)
        const keepPrefix = commonPrefixLength(prevText, nextText)
        // Clamped against what's left after the prefix: an unclamped suffix
        // match can overlap it (an identical string matches fully from BOTH
        // ends), which would otherwise make freshSuffix's length negative.
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
    // What this rebuild is about to tear down and put back, as flat text. The
    // part of it that is CHARACTER FOR CHARACTER what was already there is
    // already on the reader's screen, and re-marking it fresh would fade it in
    // a second time.
    //
    // This is the ordinary case, not a corner: the batcher hands over one
    // flush per frame (streaming-message-batcher.ts), so a single delta
    // routinely both extends the open paragraph AND starts the next one. That
    // makes `stable` land BEFORE the open paragraph while the block count also
    // grows, which is exactly the shape that reaches this path — and without a
    // skip the whole finished paragraph was removed, reinserted and re-faded
    // from `opacity: 0` the moment the paragraph after it began. That is the
    // "the sentence renders again once it finishes" report, and it is why
    // `recordBlockRuns` grew a `skip` in the first place; only the
    // mark-completing caller above was ever passing one.
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
      // One `insert_node` per BLOCK — a block's whole subtree rides along
      // inside that single operation, so this stays O(blocks), never O(words).
      editor.tf.insertNodes(newNodes as Value, { at: [stable] })
      // ONE counter threaded through every block, so the skip is consumed
      // across the block boundary rather than restarting per block.
      const skip = { remaining: carriedOver }
      newNodes.forEach((node, i) => {
        recordBlockRuns(editor, node as Node, [stable + i], generation, wordIndex, total, skip)
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

/**
 * The `animation-delay` a span should MOUNT with: this word's own wait, less
 * however much of it already went by while the run was on screen under some
 * earlier span.
 *
 * Call it once per mount and never again — `chat-fresh-text-plugin.tsx` holds
 * it in a memo keyed by the run, which is what keeps a running animation's
 * delay from being rewritten underneath it (see CHAT_FRESH_DELAY_MARK).
 *
 * This is where "already-arrived text is never left invisible" is enforced,
 * and it holds by arithmetic rather than by an event arriving. slate-react
 * keys each rendered leaf by its positional index into a decoration split
 * that is rebuilt on every delta, so these spans are remounted constantly
 * while their block streams (measured live on one 16-item list: 1918 mounts
 * for 887 animation starts). A remount used to restart the fade from
 * `opacity: 0`, so under a fast stream the restarts outran the fades and
 * words sat invisible for the rest of the turn. Subtracting the run's real
 * age makes a remount RESUME: past the wait the result is negative, which CSS
 * reads as "this animation started that long ago", and the word paints
 * mid-fade or already opaque. Clamped at the fade's own length, so the
 * furthest behind a remount can ever land is "finished".
 */
// When each run was FIRST asked for a delay — i.e. when it first reached the
// screen. Keyed by the run's `bornAt`, which is a float timestamp and so
// unique per run across every editor on the page.
//
// The origin has to be first paint rather than `bornAt` itself: the words of
// one chunk all resolve their delay inside a single React commit, and a commit
// that straddles a frame would otherwise hand each of them a DIFFERENT
// elapsed, shortening every delay but the first and collapsing the cascade
// this module exists to stage. Measuring from first paint makes the whole
// chunk share one origin however long that commit takes.
const firstPaintedAt = new Map<number, number>()
// Runs retire in tens of milliseconds; this only has to not grow without
// bound across a long session.
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

export function resumedFadeDelay(leaf: TText): number | null {
  const base = freshLeafDelay(leaf)
  if (base === null) return null
  const bornAt = (leaf as unknown as Record<string, unknown>)[CHAT_FRESH_BORN_MARK]
  if (typeof bornAt !== 'number') return base
  const now = typeof performance === 'object' ? performance.now() : Date.now()
  // Floored to whole frames: within the frame a run first painted in there is
  // nothing to resume, so the word gets its nominal delay exactly — which
  // keeps the staggered cascade the round numbers it is specified in rather
  // than a sliver less on every mount.
  const elapsed = Math.max(0, Math.floor((now - paintOriginFor(bornAt, now)) / FRAME_MS) * FRAME_MS)
  return Math.max(base - elapsed, -FADE_DURATION_MS)
}
