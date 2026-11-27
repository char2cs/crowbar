import { parsePatchFiles } from '@pierre/diffs'
import type { CodeViewItem, FileDiffMetadata } from '@pierre/diffs'

export const MAX_TURN_DIFF_PREVIEW_CHARS = 250_000

export function parseTurnDiff(
  diff: string,
  turnId: string,
  target = '',
): CodeViewItem<undefined>[] {
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
          fileDiff: bare ? unprefixed(fileDiff, target) : fileDiff,
          version,
        }
      }),
    )
  } catch {
    return []
  }
}

function unprefixed(file: FileDiffMetadata, target: string): FileDiffMetadata {
  const bare = file.name.replace(/^b\//, '')
  const name = target === `/${bare}` ? target : bare
  const prev = file.prevName?.replace(/^a\//, '')
  if (bare === file.name || (prev !== undefined && prev !== bare)) return file
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
