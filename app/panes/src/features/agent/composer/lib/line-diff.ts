import { parseDiffFromFile } from '@pierre/diffs'
import type { CodeLine } from '@/features/agent/composer/lib/ask-request'

/** Past this many characters a diff costs more than it tells: the sides are listed whole. */
const DIFF_BUDGET = 200_000

const bare = (line: string) => line.replace(/\r?\n$/, '')

/** The part of `line` that differs from `other`: what is left between their shared head and tail. */
function changed(line: string, other: string): [number, number] | undefined {
  let head = 0
  while (head < line.length && head < other.length && line[head] === other[head]) head++
  let tail = 0
  while (
    tail < line.length - head &&
    tail < other.length - head &&
    line[line.length - 1 - tail] === other[other.length - 1 - tail]
  ) {
    tail++
  }
  const end = line.length - tail
  return end > head ? [head, end] : undefined
}

/** One edit as a line diff: context, removed and added lines, numbered within the edit, a removed
 *  line and the added one that replaces it marked where they differ. */
export function diffLines(before: string, after: string): CodeLine[] {
  if (before.length + after.length > DIFF_BUDGET) return []
  const file = (contents: string) => ({ name: 'edit', contents: contents ? `${contents}\n` : '' })
  const diff = parseDiffFromFile(file(before), file(after))
  const rows: CodeLine[] = []
  for (const hunk of diff.hunks) {
    let del = hunk.deletionLineIndex
    let add = hunk.additionLineIndex
    for (const part of hunk.hunkContent) {
      if (part.type === 'context') {
        for (let i = 0; i < part.lines; i++) {
          rows.push({ text: bare(diff.additionLines[add]!), tone: 'ctx', n: add + 1 })
          add++
          del++
        }
        continue
      }
      const removed = diff.deletionLines.slice(del, del + part.deletions).map(bare)
      const added = diff.additionLines.slice(add, add + part.additions).map(bare)
      // Lines pair up by position when the block swaps as many as it removes.
      const paired = removed.length === added.length
      removed.forEach((text, i) =>
        rows.push({
          text,
          tone: 'del',
          n: del + i + 1,
          mark: paired ? changed(text, added[i]!) : undefined,
        }),
      )
      added.forEach((text, i) =>
        rows.push({
          text,
          tone: 'add',
          n: add + i + 1,
          mark: paired ? changed(text, removed[i]!) : undefined,
        }),
      )
      del += part.deletions
      add += part.additions
    }
  }
  return rows
}
