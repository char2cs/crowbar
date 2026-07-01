import ignore from 'ignore'
import type { FileEntry } from '@/features/file-system/types/app'
import {
  getDirName,
  getRelativePath,
  normalizePath,
  pathStartsWithRoot,
  stripTrailingPathSeparators,
} from '@/utils/path-helpers'

const GITIGNORE_FILE_NAME = '.gitignore'

type IgnoreMatcher = ReturnType<typeof ignore>

export interface GitIgnoreFileReference {
  path: string
  directoryPath: string
}

export interface GitIgnoreFileContent extends GitIgnoreFileReference {
  content: string
}

interface GitIgnoreRuleSet {
  directoryPath: string
  matcher: IgnoreMatcher
}

export interface FileTreeGitIgnoreRules {
  rootFolderPath: string
  ruleSets: GitIgnoreRuleSet[]
}

/**
 * Backend file paths are workspace-relative ('web/src/x.ts'); only the
 * explorer's rootFolderPath (and desktop-heritage trees) use absolute paths.
 * Workspace-relative paths always belong to the active root.
 */
export const isWorkspaceRelativePath = (path: string): boolean =>
  !path.startsWith('/') && !path.startsWith('\\') && !/^[A-Za-z]:[\\/]/.test(path)

/** The owning directory of a root-level .gitignore in a relative tree is '' (or '.'). */
const isRootDirectoryPath = (directoryPath: string): boolean => {
  const normalized = normalizePath(stripTrailingPathSeparators(directoryPath))
  return normalized === '' || normalized === '.'
}

const isPathInRootScope = (path: string, rootFolderPath: string): boolean =>
  isWorkspaceRelativePath(path) || pathStartsWithRoot(path, rootFolderPath)

const pathDepth = (path: string): number =>
  normalizePath(stripTrailingPathSeparators(path)).split('/').filter(Boolean).length

const compareIgnoreReferences = (
  left: GitIgnoreFileReference,
  right: GitIgnoreFileReference,
): number => {
  const depthDelta = pathDepth(left.directoryPath) - pathDepth(right.directoryPath)
  if (depthDelta !== 0) return depthDelta
  return normalizePath(left.directoryPath).localeCompare(normalizePath(right.directoryPath))
}

function addGitIgnoreContent(matcher: IgnoreMatcher, content: string): void {
  for (const line of content.split(/\r?\n/)) {
    try {
      matcher.add(line)
    } catch {
      // Keep the rest of the file usable if a single malformed pattern is present.
    }
  }
}

export function collectGitIgnoreFileReferences(
  files: FileEntry[],
  rootFolderPath: string | undefined,
): GitIgnoreFileReference[] {
  if (!rootFolderPath) return []

  const references = new Map<string, GitIgnoreFileReference>()
  // Backend file paths are workspace-relative; rootFolderPath is a synthetic
  // display prefix. Accept relative paths as in-root — only absolute paths
  // from another root are filtered. (The old prefix check rejected every real
  // tree entry, and the synthetic root reference 404'd against the backend.)
  const isInRoot = (path: string) =>
    !path.startsWith('/') || pathStartsWithRoot(path, rootFolderPath)
  const addReference = (path: string) => {
    if (!isInRoot(path)) return

    const normalizedPath = normalizePath(stripTrailingPathSeparators(path))
    references.set(normalizedPath, {
      path,
      directoryPath: getDirName(path),
    })
  }

  const walk = (entries: FileEntry[]) => {
    for (const entry of entries) {
      if (entry.name === GITIGNORE_FILE_NAME && !entry.isDir) {
        addReference(entry.path)
      }

      if (entry.children) {
        walk(entry.children)
      }
    }
  }

  walk(files)

  // Reference only the .gitignore files actually present in the loaded tree. We
  // deliberately do NOT synthesize a conventional root .gitignore: the backend
  // file tree includes dotfiles, so a real root .gitignore is always surfaced by
  // the walk above once the root level loads (it loads first). Synthesizing one
  // when the project root has none — common for the project-home workspace whose
  // root is the bare project directory — fetched a non-existent file and 404'd on
  // every load.
  return [...references.values()].sort(compareIgnoreReferences)
}

export function createFileTreeGitIgnoreRules(
  rootFolderPath: string | undefined,
  ignoreFiles: GitIgnoreFileContent[],
): FileTreeGitIgnoreRules | null {
  if (!rootFolderPath || ignoreFiles.length === 0) return null

  const ruleSets = ignoreFiles
    .filter(
      (file) =>
        !file.directoryPath.startsWith('/') ||
        pathStartsWithRoot(file.directoryPath, rootFolderPath),
    )
    .sort(compareIgnoreReferences)
    .map((file) => {
      const matcher = ignore({ allowRelativePaths: true })
      addGitIgnoreContent(matcher, file.content)

      return {
        directoryPath: file.directoryPath,
        matcher,
      }
    })

  if (ruleSets.length === 0) return null

  return {
    rootFolderPath,
    ruleSets,
  }
}

function toMatcherPath(fullPath: string, directoryPath: string, isDir: boolean): string | null {
  let relative: string

  if (isRootDirectoryPath(directoryPath)) {
    // Root .gitignore over a workspace-relative tree: the entry path already
    // is the matcher path. Absolute paths cannot be relativized against ''.
    if (!isWorkspaceRelativePath(fullPath)) return null
    relative = fullPath
  } else if (pathStartsWithRoot(fullPath, directoryPath)) {
    relative = getRelativePath(fullPath, directoryPath)
  } else {
    return null
  }

  if (!relative || relative.trim() === '') return null

  relative = normalizePath(relative)
  if (isDir && !relative.endsWith('/')) {
    relative += '/'
  }

  return relative
}

function isPathIgnoredByOwnRules(
  rules: FileTreeGitIgnoreRules | null,
  fullPath: string,
  isDir: boolean,
): boolean {
  if (!rules || !isPathInRootScope(fullPath, rules.rootFolderPath)) return false

  let rootRelative = isWorkspaceRelativePath(fullPath)
    ? fullPath
    : getRelativePath(fullPath, rules.rootFolderPath)
  if (!rootRelative || rootRelative.trim() === '') return false
  rootRelative = normalizePath(rootRelative)
  if (rootRelative === '.git' || rootRelative === '.git/') return false

  let ignored = false

  for (const ruleSet of rules.ruleSets) {
    const matcherPath = toMatcherPath(fullPath, ruleSet.directoryPath, isDir)
    if (!matcherPath) continue

    const result = ruleSet.matcher.test(matcherPath)
    if (result.ignored) {
      ignored = true
    } else if (result.unignored) {
      ignored = false
    }
  }

  return ignored
}

function getAncestorDirectoryPaths(fullPath: string, rootFolderPath: string): string[] {
  const ancestors: string[] = []
  const normalizedRootPath = normalizePath(stripTrailingPathSeparators(rootFolderPath))
  let currentPath = getDirName(fullPath)

  while (currentPath && isPathInRootScope(currentPath, rootFolderPath)) {
    if (normalizePath(stripTrailingPathSeparators(currentPath)) === normalizedRootPath) {
      break
    }

    ancestors.unshift(currentPath)
    currentPath = getDirName(currentPath)
  }

  return ancestors
}

export function isPathGitIgnoredByFileTreeRules(
  rules: FileTreeGitIgnoreRules | null,
  fullPath: string,
  isDir: boolean,
): boolean {
  if (!rules || !isPathInRootScope(fullPath, rules.rootFolderPath)) return false

  for (const ancestorPath of getAncestorDirectoryPaths(fullPath, rules.rootFolderPath)) {
    if (isPathIgnoredByOwnRules(rules, ancestorPath, true)) {
      return true
    }
  }

  return isPathIgnoredByOwnRules(rules, fullPath, isDir)
}
