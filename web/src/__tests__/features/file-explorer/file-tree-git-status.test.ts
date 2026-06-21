import { describe, expect, test } from 'vitest'
import type { FileEntry } from '@/features/file-system/types/app'
import type { GitFile, GitStatus } from '@/features/git/types/git-types'
import {
  createFileTreeGitStatusLookup,
  getFileTreeEntryGitStatusDecoration,
  getFileTreeGitStatusDecoration,
  resolveActiveWorkspaceGitStatus,
} from '@/features/file-explorer/file-explorer/lib/file-tree-git-status'

const gitFile = (path: string, status: GitFile['status'], staged = false): GitFile => ({
  path,
  status,
  staged,
})

const fileEntry = (path: string, isDir = false): FileEntry => ({
  name: path.split('/').pop() ?? path,
  path,
  isDir,
})

describe('getFileTreeGitStatusDecoration', () => {
  test('maps modified files to staged and unstaged colors', () => {
    expect(getFileTreeGitStatusDecoration(gitFile('src/app.ts', 'modified'))).toEqual({
      colorClassName: 'text-git-modified',
      label: 'Modified',
      statusLetter: 'M',
    })

    expect(getFileTreeGitStatusDecoration(gitFile('src/app.ts', 'modified', true))).toEqual({
      colorClassName: 'text-git-modified-staged',
      label: 'Modified (staged)',
      statusLetter: 'M',
    })
  })

  test('maps non-modified statuses to their file tree colors', () => {
    expect(getFileTreeGitStatusDecoration(gitFile('added.ts', 'added'))).toEqual({
      colorClassName: 'text-git-added',
      label: 'Added',
      statusLetter: 'A',
    })
    expect(getFileTreeGitStatusDecoration(gitFile('deleted.ts', 'deleted'))).toEqual({
      colorClassName: 'text-git-deleted',
      label: 'Deleted',
      statusLetter: 'D',
    })
    expect(getFileTreeGitStatusDecoration(gitFile('untracked.ts', 'untracked'))).toEqual({
      colorClassName: 'text-git-untracked',
      label: 'Untracked',
      statusLetter: 'U',
    })
    expect(getFileTreeGitStatusDecoration(gitFile('renamed.ts', 'renamed'))).toEqual({
      colorClassName: 'text-git-renamed',
      label: 'Renamed',
      statusLetter: 'R',
    })
  })
})

describe('file tree git status lookup', () => {
  test('keeps exact file status and inherited directory status separate', () => {
    const gitStatus: GitStatus = {
      branch: 'main',
      ahead: 0,
      behind: 0,
      files: [gitFile('src/app.ts', 'modified'), gitFile('docs/readme.md', 'added')],
    }

    const lookup = createFileTreeGitStatusLookup(gitStatus)

    expect(
      getFileTreeEntryGitStatusDecoration(fileEntry('/workspace/src/app.ts'), '/workspace', lookup),
    ).toEqual({ colorClassName: 'text-git-modified', label: 'Modified', statusLetter: 'M' })

    expect(
      getFileTreeEntryGitStatusDecoration(fileEntry('/workspace/src', true), '/workspace', lookup),
    ).toEqual({ colorClassName: 'text-git-modified', label: 'Modified', statusLetter: 'M' })

    expect(
      getFileTreeEntryGitStatusDecoration(fileEntry('/workspace/docs', true), '/workspace', lookup),
    ).toEqual({ colorClassName: 'text-git-added', label: 'Added', statusLetter: 'A' })
  })

  test('uses the highest priority descendant status for directories', () => {
    const lookup = createFileTreeGitStatusLookup({
      branch: 'main',
      ahead: 0,
      behind: 0,
      files: [
        gitFile('src/new.ts', 'untracked'),
        gitFile('src/renamed.ts', 'renamed'),
        gitFile('src/deleted.ts', 'deleted'),
        gitFile('src/modified.ts', 'modified'),
      ],
    })

    expect(
      getFileTreeEntryGitStatusDecoration(fileEntry('/workspace/src', true), '/workspace', lookup),
    ).toEqual({ colorClassName: 'text-git-deleted', label: 'Deleted', statusLetter: 'D' })
  })

  test('returns null without a root path or matching status', () => {
    const lookup = createFileTreeGitStatusLookup({
      branch: 'main',
      ahead: 0,
      behind: 0,
      files: [gitFile('src/app.ts', 'modified')],
    })

    expect(
      getFileTreeEntryGitStatusDecoration(fileEntry('/workspace/src/app.ts'), undefined, lookup),
    ).toBeNull()
    expect(
      getFileTreeEntryGitStatusDecoration(
        fileEntry('/workspace/src/other.ts'),
        '/workspace',
        lookup,
      ),
    ).toBeNull()
  })

  // Regression: the real app addresses file-tree entries by WORKSPACE-RELATIVE
  // paths ("README.md", "api/x.go") while rootFolderPath is the synthetic
  // `/repos/<repoId>` mock-era prefix — a different id space. The git status the
  // backend returns is keyed by the same workspace-relative paths. These resolve
  // correctly even though the file path never starts with rootFolderPath
  // (getRelativePath returns the path unchanged when there is no prefix match).
  // The previous code gated this behind `getWorkspaceRootForPath === rootFolderPath`
  // which was always false, silently disabling every decoration.
  test('resolves decorations for workspace-relative paths under a synthetic /repos root', () => {
    const root = '/repos/81883222-d45f-44ca-80ed-9550d5228441'
    const lookup = createFileTreeGitStatusLookup({
      branch: 'epoch/first-pr',
      ahead: 0,
      behind: 1,
      files: [
        gitFile('README.md', 'modified'),
        gitFile('api/internal/api/container.go', 'modified'),
      ],
    })

    expect(getFileTreeEntryGitStatusDecoration(fileEntry('README.md'), root, lookup)).toEqual({
      colorClassName: 'text-git-modified',
      label: 'Modified',
      statusLetter: 'M',
    })
    expect(
      getFileTreeEntryGitStatusDecoration(
        fileEntry('api/internal/api/container.go'),
        root,
        lookup,
      ),
    ).toEqual({ colorClassName: 'text-git-modified', label: 'Modified', statusLetter: 'M' })
    // The parent folder inherits the descendant's status (folder tinting).
    expect(getFileTreeEntryGitStatusDecoration(fileEntry('api', true), root, lookup)).toEqual({
      colorClassName: 'text-git-modified',
      label: 'Modified',
      statusLetter: 'M',
    })
  })
})

describe('resolveActiveWorkspaceGitStatus', () => {
  const status: GitStatus = {
    branch: 'epoch/first-pr',
    ahead: 0,
    behind: 1,
    files: [gitFile('README.md', 'modified')],
  }
  const wsId = 'd2e0a0de-dbee-4fc3-a333-2cac9b6aeff3'

  test('applies the status when the git store loaded the active workspace', () => {
    expect(resolveActiveWorkspaceGitStatus(status, wsId, wsId)).toBe(status)
  })

  test('rejects a status loaded for a different workspace', () => {
    expect(resolveActiveWorkspaceGitStatus(status, 'other-ws-id', wsId)).toBeNull()
  })

  test('returns null when any input is missing', () => {
    expect(resolveActiveWorkspaceGitStatus(null, wsId, wsId)).toBeNull()
    expect(resolveActiveWorkspaceGitStatus(status, null, wsId)).toBeNull()
    expect(resolveActiveWorkspaceGitStatus(status, wsId, null)).toBeNull()
  })
})
