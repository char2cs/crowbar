import { parsePatchFiles } from '@pierre/diffs'
import type { CodeViewItem } from '@pierre/diffs'

/** Keep parser and syntax-highlighting work bounded for a live preview. */
export const MAX_TURN_DIFF_PREVIEW_CHARS = 250_000

/**
 * Parse the newest complete diff snapshot into provider-neutral file items.
 * The cache fingerprint changes with the content, while item IDs remain stable
 * within a turn so CodeView can update the same file as the snapshot advances.
 */
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
          fileDiff,
          version,
        }
      }),
    )
  } catch {
    // A provider's partial or unfamiliar patch remains readable via the raw
    // fallback in the component; a parser failure must not hide the activity.
    return []
  }
}

function fingerprint(text: string): number {
  let hash = 0x811c9dc5
  for (let i = 0; i < text.length; i++) {
    hash ^= text.charCodeAt(i)
    hash = Math.imul(hash, 0x01000193)
  }
  return hash >>> 0
}
