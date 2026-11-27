import type { DecoratedRange } from '@platejs/slate'
import type { NodeEntry, Path, Value } from 'platejs'
import type { PlateEditor } from 'platejs/react'
import {
  commonPrefixLength,
  commonSuffixLength,
  flattenText,
  lastLeaf,
  leafDivergences,
  markKeys,
  nodesEqual,
  ownPropsEqual,
  stableBlockCount,
  trailingTextDivergence,
  type LeafDiff,
  type Node,
} from '@/features/agent/transcript/plate/streaming-value-blocks'
import { splitIntoWords, staggerDelay } from '@/features/agent/transcript/plate/fresh-word-stagger'

export const CHAT_FRESH_MARK = 'chatFresh'
export const CHAT_FRESH_DELAY_MARK = 'chatFreshDelay'
export const CHAT_FRESH_BORN_MARK = 'chatFreshBornAt'
const CHAT_FRESH_HELD = 'chatFreshHeld'
export const CHAT_FRESH_WORD_INDEX_MARK = 'chatFreshWordIndex'
export const CHAT_FRESH_WORD_TOTAL_MARK = 'chatFreshWordTotal'

const SCROLL_LEAD_MS = 150

const WORD_SPLIT_CAP = 80

interface FreshRun {
  generation: number
  path: Path
  start: number
  end: number
  wordOffset: number
  totalWords: number
  bornAt: number
}

const freshRuns = new WeakMap<PlateEditor, FreshRun[]>()
const settledGenerations = new WeakMap<PlateEditor, Set<number>>()
const freshGenerations = new WeakMap<PlateEditor, number>()

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

const pendingCleanup = new WeakMap<PlateEditor, number>()

function scheduleFadeCleanup(editor: PlateEditor): void {
  if (typeof requestAnimationFrame !== 'function') return
  if (pendingCleanup.has(editor)) return
  pendingCleanup.set(
    editor,
    requestAnimationFrame(() => {
      pendingCleanup.delete(editor)
      editor.api.redecorate?.()
    }),
  )
}

export function settleFreshGeneration(editor: PlateEditor, generation: number): void {
  const settled = settledGenerations.get(editor) ?? new Set<number>()
  if (settled.has(generation)) return
  settled.add(generation)
  settledGenerations.set(editor, settled)
  scheduleFadeCleanup(editor)
}

const settledWordCounts = new WeakMap<PlateEditor, Map<number, Set<number>>>()

export function settleFreshWord(
  editor: PlateEditor,
  generation: number,
  wordIndex: number,
  totalWords: number,
): void {
  const byGeneration = settledWordCounts.get(editor) ?? new Map<number, Set<number>>()
  const words = byGeneration.get(generation) ?? new Set<number>()
  words.add(wordIndex)
  byGeneration.set(generation, words)
  settledWordCounts.set(editor, byGeneration)
  if (words.size >= totalWords) settleFreshGeneration(editor, generation)
}

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
    if (run.end > text.length || run.start >= run.end) continue
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

const knownStablePrefix = new WeakMap<PlateEditor, number>()

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

function recordBlockRuns(
  editor: PlateEditor,
  node: Node,
  path: Path,
  generation: number,
  wordIndex: { current: number },
  total: number,
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

function replaceChildTail(editor: PlateEditor, prevBlock: Node, nextBlock: Node, at: Path) {
  const prev = prevBlock.children
  const next = nextBlock.children
  if (!prev || !next || !ownPropsEqual(prevBlock, nextBlock)) return false
  let kept = 0
  while (kept < Math.min(prev.length, next.length) && nodesEqual(prev[kept], next[kept])) kept++
  if (kept === 0) return false
  for (let i = prev.length - 1; i >= kept; i--) editor.tf.removeNodes({ at: [...at, i] })
  const added = next.slice(kept)
  if (added.length === 0) return true
  editor.tf.insertNodes(added as Value, { at: [...at, kept] })
  const prevText = prev.slice(kept).map(flattenText).join('')
  const nextText = added.map(flattenText).join('')
  const skip = { remaining: commonPrefixLength(prevText, nextText) }
  const total = splitIntoWords(nextText.slice(skip.remaining)).length
  const generation = nextFreshGeneration(editor)
  const wordIndex = { current: 0 }
  added.forEach((child, i) => {
    recordBlockRuns(editor, child, [...at, kept + i], generation, wordIndex, total, skip)
  })
  return true
}

export function applyStreamedValue(editor: PlateEditor, next: Value): void {
  const prev = editor.children as Value
  const startAt = Math.min(knownStablePrefix.get(editor) ?? 0, prev.length, next.length)
  const stable = startAt + stableBlockCount(prev.slice(startAt), next.slice(startAt))
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
        if (staleLength === 0 && divergence.replacement === '') return
        const endPoint = editor.api.end([stable])
        if (endPoint) {
          if (staleLength > 0) {
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
        const leafDiffs: LeafDiff[] = []
        if (leafDivergences(prevBlock, nextBlock, [stable], leafDiffs) && leafDiffs.length > 0) {
          pruneRuns(editor, stable)
          patchLeavesInPlace(editor, leafDiffs)
          return
        }
        pruneRuns(editor, stable)
        if (replaceChildTail(editor, prevBlock, nextBlock, [stable])) return
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
      const skip = { remaining: carriedOver }
      newNodes.forEach((node, i) => {
        recordBlockRuns(editor, node as Node, [stable + i], generation, wordIndex, total, skip)
      })
    }
  })
}
