import { parsePatchFiles } from '@pierre/diffs'
import type { CodeViewItem, FileDiffMetadata } from '@pierre/diffs'

/** Keep parser and syntax-highlighting work bounded for a live preview. */
export const MAX_TURN_DIFF_PREVIEW_CHARS = 250_000

/** The fingerprint changes with content while item IDs stay stable within a turn,
 *  so CodeView updates the same file as the snapshot advances. */
export function parseTurnDiff(diff: string, turnId: string): CodeViewItem<undefined>[] {
  if (!diff || diff.length > MAX_TURN_DIFF_PREVIEW_CHARS) return []

  try {
    const version = fingerprint(diff)
    const patches = parsePatchFiles(diff, `${turnId}:${version}`, true)
    const bare = !/^diff --git /m.test(diff)
    let fileIndex = 0
    return patches.flatMap((patch, patchIndex) =>
      patch.files.map((fileDiff) => {
        const index = fileIndex++
        return {
          id: `${turnId}:${patchIndex}:${index}`,
          type: 'diff' as const,
          fileDiff: bare ? unprefixed(fileDiff) : fileDiff,
          version,
        }
      }),
    )
  } catch {
    // The component falls back to the raw diff; a parse failure must not hide it.
    return []
  }
}

/** A tool's edit arrives as a bare unified diff, whose `a/`/`b/` names the parser
 *  keeps, so every edit would read as a rename from a/ to b/. */
function unprefixed(file: FileDiffMetadata): FileDiffMetadata {
  const name = file.name.replace(/^b\//, '')
  const prev = file.prevName?.replace(/^a\//, '')
  if (name === file.name || (prev !== undefined && prev !== name)) return file
  return { ...file, name, prevName: undefined, type: prev === undefined ? file.type : 'change' }
}

function fingerprint(text: string): number {
  let hash = 0x811c9dc5
  for (let i = 0; i < text.length; i++) {
    hash ^= text.charCodeAt(i)
    hash = Math.imul(hash, 0x01000193)
  }
  return hash >>> 0
}
