import type { Value } from 'platejs'
import type { PlateEditor } from 'platejs/react'

type Node = { text?: string; children?: Node[] } & Record<string, unknown>

/** The mark `chat-fresh-text-plugin.tsx` renders as a fade-in — set here,
 *  nowhere else, always on text a full markdown parse already produced (see
 *  that file for why this never streams anything bare and restyles it
 *  after). Owned here, not there: the plugin depends on this module's
 *  vocabulary, not the other way round. */
export const CHAT_FRESH_MARK = 'chatFresh'
/** This leaf's `animation-delay`, in ms — see `staggerDelay` below. */
export const CHAT_FRESH_DELAY_MARK = 'chatFreshDelay'

// Keys equality must look past: `NodeIdPlugin` (on by default — see
// chat-composer-plugins.ts) stamps a fresh random id onto every block it
// normalizes, so two parses of identical markdown never carry the same one —
// it is Plate's own "not content" prop (`isMetadataProp` flags exactly this
// key). The two CHAT_FRESH_* keys are ours: transient UI state, not content,
// and a leaf still mid-fade must still compare equal to its plain-parsed
// self, or every patch while one is animating would miss the fast path.
const IGNORED_KEYS = new Set([CHAT_FRESH_MARK, CHAT_FRESH_DELAY_MARK, 'id'])

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
// running ResizeObserver-driven loop) starts converging on the new bottom
// immediately — concurrently with, not after, the word cascade below. This
// flat head start on every word in a chunk (added on top of staggerDelay, not
// inside it — that function's own tests assert its bare per-word values) lets
// the scroll get underway before text starts materializing, so a chunk reads
// as "settles into view, then fills in" rather than everything at once.
const SCROLL_LEAD_MS = 150

/** Splits text into whitespace-preserving chunks — concatenating the result
 *  reconstructs the original string exactly, including leading/repeated
 *  whitespace, unlike a plain `.split(' ')`. */
export function splitIntoWords(text: string): string[] {
  return text.match(/\s*\S+\s*/g) ?? [text]
}

/** How many word-chunks `markFresh` will split `node` into — needed BEFORE
 *  building the split tree, since every word in one insertion shares a
 *  stagger step sized off the total (see `staggerDelay`). */
function countWords(node: Node): number {
  if (typeof node.text === 'string') return splitIntoWords(node.text).length
  if (!node.children) return 0
  return node.children.reduce((sum, child) => sum + countWords(child), 0)
}

// The mark's VALUE is a per-call generation counter, not `true`: Slate
// normalization merges adjacent leaves with IDENTICAL marks, so two fresh
// runs inserted a token apart would coalesce into one the instant they
// became neighbors — which is every time, since a new run always lands right
// next to the last one. A distinct generation per run (word delay alone
// isn't enough — see below) keeps runs apart until EACH settles on its own,
// which is what lets several trailing words be mid-fade at once instead of
// one fade whose span keeps absorbing the next.
const freshGenerations = new WeakMap<PlateEditor, number>()
function nextFreshGeneration(editor: PlateEditor): number {
  const next = (freshGenerations.get(editor) ?? 0) + 1
  freshGenerations.set(editor, next)
  return next
}

// How many of `editor.children`'s LEADING blocks are already confirmed to
// match a fresh reparse, as of the last call. `applyStreamedValue` never
// touches anything before its own `stable` boundary once computed — so a
// block confirmed stable stays stable for the rest of this editor's life,
// and re-canonicalizing/re-comparing it on every later token is exactly the
// "touch everything, not just what changed" cost this module exists to
// avoid. Left unchecked this turns one streamed message into O(length²)
// work: by message end, EVERY already-settled paragraph gets walked again
// on EVERY remaining token. Measured live: this is what was actually
// starving the scroll and fade animations of frames on anything longer than
// a few paragraphs, not a flaw in either animation itself.
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

/**
 * Deep-clones `node`, splitting every text leaf into per-word leaves and
 * marking each fresh with its own stagger delay — used for content that
 * arrived as a whole new block, not a trailing append (see the fallback path
 * below), so it cascades in the same way a trailing append does.
 *
 * Returns an ARRAY: a leaf that splits into several words produces several
 * SIBLING leaves, not one leaf holding both `text` and `children` (which
 * Slate does not have a node shape for). Callers flatten accordingly.
 *
 * `wordIndex` is threaded through the whole recursive walk (not reset per
 * leaf) so a heading followed by its body text staggers as one continuous
 * cascade, matching how one append does. `total` is the word count across
 * the WHOLE insertion (every top-level node this call is part of), not just
 * this one node — computed once up front via `countWords`, so the stagger
 * step is sized correctly from the first word.
 */
function markFresh(
  node: Node,
  generation: number,
  wordIndex: { current: number },
  total: number,
): Node[] {
  if (typeof node.text === 'string') {
    return splitIntoWords(node.text).map((word) => ({
      ...node,
      text: word,
      [CHAT_FRESH_MARK]: generation,
      [CHAT_FRESH_DELAY_MARK]: SCROLL_LEAD_MS + staggerDelay(wordIndex.current++, total),
    }))
  }
  if (!node.children) return [node]
  return [
    {
      ...node,
      children: node.children.flatMap((child) => markFresh(child, generation, wordIndex, total)),
    },
  ]
}

/**
 * `prev`, as it will look once every still-fading leaf has settled — i.e.
 * exactly what a fresh reparse of the same text already looks like: fresh
 * marks stripped, and the extra leaf boundaries the per-word split leaves
 * behind collapsed back into one.
 *
 * Diffing against this instead of the live tree directly is what keeps the
 * fast append path available while text is mid-fade: without it, the several
 * leaves one staggered chunk splits into would make every following token —
 * arriving well within that chunk's ~500ms cascade — see a leaf COUNT that no
 * longer matches a plain reparse, and fall back to a full block replace for
 * as long as anything nearby is still fading. It does not change what gets
 * inserted or where — `stable`/`editor.api.end` still read the real,
 * uncanonicalized editor — only what counts as "unchanged" for deciding it.
 */
function canonicalize(node: Node): Node {
  if (typeof node.text === 'string') {
    let stripped = node
    for (const key of IGNORED_KEYS) {
      if (!(key in stripped)) continue
      const { [key]: _omit, ...rest } = stripped
      stripped = rest
    }
    return stripped
  }
  if (!node.children) return node
  const merged: Node[] = []
  for (const raw of node.children) {
    const child = canonicalize(raw)
    const last = merged[merged.length - 1]
    if (
      last &&
      typeof last.text === 'string' &&
      typeof child.text === 'string' &&
      ownPropsEqual(last, child)
    ) {
      merged[merged.length - 1] = { ...last, text: (last.text as string) + (child.text as string) }
    } else {
      merged.push(child)
    }
  }
  return { ...node, children: merged }
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
 * case — a paragraph or code line growing token by token — becomes a small
 * run of inserted text nodes rather than a node replacement.
 *
 * Whatever text actually lands — the appended run, or a whole new block —
 * is split into words and each marked `CHAT_FRESH_MARK` (with its own
 * `CHAT_FRESH_DELAY_MARK`) so it cascades in (chat-fresh-text-plugin.tsx)
 * instead of popping onto the screen already at full opacity — independent
 * of how large a chunk the transport happened to deliver in one delta.
 *
 * Only canonicalizes the TAIL from `knownStablePrefix` onward, not every
 * block — see that WeakMap's own comment for why re-canonicalizing the
 * whole document on every token made this O(message length²) in practice.
 */
export function applyStreamedValue(editor: PlateEditor, next: Value): void {
  const prev = editor.children as Value
  const startAt = Math.min(knownStablePrefix.get(editor) ?? 0, prev.length, next.length)
  const comparableTail = prev.slice(startAt).map((block) => canonicalize(block as Node)) as Value
  const stable = startAt + stableBlockCount(comparableTail, next.slice(startAt))
  knownStablePrefix.set(editor, stable)
  if (stable === prev.length && stable === next.length) return

  editor.tf.withoutNormalizing(() => {
    if (stable === prev.length - 1 && stable === next.length - 1) {
      const comparableBlock = comparableTail[stable - startAt] as Node
      const nextBlock = next[stable] as Node
      const divergence = trailingTextDivergence(comparableBlock, nextBlock)
      if (divergence) {
        const prevText = lastLeaf(comparableBlock).text as string
        const staleLength = prevText.length - divergence.keep
        if (staleLength === 0 && divergence.replacement === '') return // truly unchanged
        const endPoint = editor.api.end([stable])
        if (endPoint) {
          // A pure append (staleLength === 0) needs no delete — the common
          // case, unchanged from before this generalized. Anything reached
          // here with staleLength > 0 is what a pure append couldn't
          // express: the terminating hook's own text reconciled part of the
          // tail to something DIFFERENT, not just longer (closeAssistantTurn,
          // provider-agnostic). Removing only the diverging suffix — never
          // the shared prefix before it — is what keeps that prefix's fade
          // state untouched instead of re-triggering the whole block's cascade.
          if (staleLength > 0) {
            editor.tf.delete({ at: endPoint, distance: staleLength, unit: 'character', reverse: true })
          }
          // Not `insertText`: that extends the EXISTING leaf in place, which
          // cannot carry a mark the neighboring, already-settled text lacks.
          // New leaves are what let each word fade independently and then,
          // once its mark clears, get merged back by Slate's normal "adjacent
          // leaves with identical marks merge" rule.
          if (divergence.replacement !== '') {
            // Re-read after the delete above: deleting a range shifts every
            // point after it, so the pre-delete `endPoint` no longer names
            // the block's end once something was removed.
            const insertAt = staleLength > 0 ? editor.api.end([stable]) : endPoint
            if (insertAt) {
              const marks = lastLeaf(nextBlock)
              const generation = nextFreshGeneration(editor)
              const words = splitIntoWords(divergence.replacement)
              editor.tf.insertNodes(
                words.map((word, i) => ({
                  ...marks,
                  text: word,
                  [CHAT_FRESH_MARK]: generation,
                  [CHAT_FRESH_DELAY_MARK]: SCROLL_LEAD_MS + staggerDelay(i, words.length),
                })),
                { at: insertAt },
              )
            }
          }
          return
        }
      }
    }
    let removing = prev.length - stable
    while (removing-- > 0) editor.tf.removeNodes({ at: [stable] })
    if (stable < next.length) {
      const newNodes = next.slice(stable)
      const generation = nextFreshGeneration(editor)
      const wordIndex = { current: 0 }
      const total = newNodes.reduce((sum, node) => sum + countWords(node as Node), 0)
      editor.tf.insertNodes(
        newNodes.flatMap((node) => markFresh(node as Node, generation, wordIndex, total)) as Value,
        { at: [stable] },
      )
    }
  })
}
