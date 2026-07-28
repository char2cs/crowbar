import type { GitDiff, GitDiffLine } from './git-types'

export interface DiffLineWithIndex extends GitDiffLine {
  diffIndex: number
}

export interface ParsedHunk {
  header: GitDiffLine
  lines: DiffLineWithIndex[]
  id: number
}

export interface ImageContainerProps {
  label: string
  labelColor: string
  base64?: string
  alt: string
  zoom: number
}

export interface ImageDiffViewerProps {
  diff: GitDiff
  fileName: string
}

export interface MultiFileDiff {
  title?: string
  repoPath?: string
  commitHash: string
  commitMessage?: string
  commitDescription?: string
  commitAuthor?: string
  commitDate?: string
  files: GitDiff[]
  totalFiles: number
  totalAdditions: number
  totalDeletions: number
  fileKeys?: string[]
  initiallyExpandedFileKey?: string
  isLoading?: boolean
}
