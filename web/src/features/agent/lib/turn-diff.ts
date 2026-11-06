import { parsePatchFiles } from '@pierre/diffs'
import type { CodeViewItem } from '@pierre/diffs'

/** Keep parser and syntax-highlighting work bounded for a live preview. */
export const MAX_TURN_DIFF_PREVIEW_CHARS = 250_000

/** The fingerprint changes with content while item IDs stay stable within a turn,
 *  so CodeView updates the same file as the snapshot advances. */
export function parseTurnDiff(diff: string, turnId: string): CodeViewItem<undefined>[] {
  if (!diff || diff.length > MAX_TURN_DIFF_PREVIEW_CHARS) return []

  try {
    const version = fingerprint(diff)
    const patches = parsePatchFiles(withGitHeaders(diff), `${turnId}:${version}`, true)
    let fileIndex = 0
    return patches.flatMap((patch, patchIndex) =>
      patch.files.map((fileDiff) => {
        const index = fileIndex++
        return {
          id: `${turnId}:${patchIndex}:${index}`,
          type: 'diff' as const,
          fileDiff,
          version,
        }
      }),
    )
  } catch {
    // The component falls back to the raw diff; a parse failure must not hide it.
    return []
  }
}

const HUNK = /^@@ -\d+(?:,(\d+))? \+\d+(?:,(\d+))? @@/
const side = (header: string, prefix: string) => {
  const path = header.slice(4).split('\t')[0] ?? ''
  return path === '/dev/null' ? '' : path.replace(prefix, '')
}

/** A tool's edit arrives as a bare unified diff, whose `a/`/`b/` names the parser
 *  keeps (reading every edit as a rename) unless a git header says what they are.
 *  Hunks are walked by their counts, so a removed `-- x` line is never a header. */
function withGitHeaders(diff: string): string {
  if (/^diff --git /m.test(diff)) return diff
  const lines = diff.split('\n')
  const out: string[] = []
  let left = { old: 0, new: 0 }
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i]!
    const next = lines[i + 1] ?? ''
    if (left.old > 0 || left.new > 0) {
      if (!line.startsWith('+')) left.old--
      if (!line.startsWith('-')) left.new--
    } else if (line.startsWith('--- ') && next.startsWith('+++ ')) {
      const from = side(line, 'a/')
      const to = side(next, 'b/')
      out.push(`diff --git a/${from || to} b/${to || from}`)
    } else {
      const hunk = HUNK.exec(line)
      if (hunk) left = { old: Number(hunk[1] ?? 1), new: Number(hunk[2] ?? 1) }
    }
    out.push(line)
  }
  return out.join('\n')
}

function fingerprint(text: string): number {
  let hash = 0x811c9dc5
  for (let i = 0; i < text.length; i++) {
    hash ^= text.charCodeAt(i)
    hash = Math.imul(hash, 0x01000193)
  }
  return hash >>> 0
}
