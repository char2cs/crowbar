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
    let fileIndex = 0
    return patches.flatMap((patch, patchIndex) =>
      patch.files.map((fileDiff) => {
        const index = fileIndex++
        return {
          id: `${turnId}:${patchIndex}:${index}`,
          type: 'diff' as const,
          fileDiff: withoutGitPrefixes(fileDiff),
          version,
        }
      }),
    )
  } catch {
    // The component falls back to the raw diff; a parse failure must not hide it.
    return []
  }
}

// Without a `diff --git` line the parser keeps git's a/ b/ prefixes, so an edit of one file
// reads as a rename from a/x to b/x; the daemon's per-edit diffs are written that way.
function withoutGitPrefixes(file: FileDiffMetadata): FileDiffMetadata {
  if (!file.prevName?.startsWith('a/') || !file.name.startsWith('b/')) return file
  const name = file.name.slice(2)
  const prevName = file.prevName.slice(2)
  if (prevName !== name) return { ...file, name, prevName }
  return { ...file, name, prevName: undefined, type: 'change' }
}

function fingerprint(text: string): number {
  let hash = 0x811c9dc5
  for (let i = 0; i < text.length; i++) {
    hash ^= text.charCodeAt(i)
    hash = Math.imul(hash, 0x01000193)
  }
  return hash >>> 0
}
