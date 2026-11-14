import type { GitDiff } from '../types/git-types'

export const getImgSrc = (base64: string | undefined) =>
  base64 ? `data:image/*;base64,${base64}` : undefined

export function getFileStatus(diff: GitDiff): string {
  if (diff.is_new) return 'added'
  if (diff.is_deleted) return 'deleted'
  if (diff.is_renamed) return 'renamed'
  return 'modified'
}
