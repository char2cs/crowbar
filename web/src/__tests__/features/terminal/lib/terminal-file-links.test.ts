import { describe, it, expect } from 'vitest'
import {
  parseFileLinkCandidates,
  resolveFileLinkPath,
  workspaceRelativePath,
} from '@/features/terminal/lib/terminal-file-links'

describe('parseFileLinkCandidates', () => {
  it('matches a repo-relative path with a line suffix', () => {
    const [c] = parseFileLinkCandidates('error in web/src/foo/bar.ts:12 something')
    expect(c).toMatchObject({ text: 'web/src/foo/bar.ts:12', path: 'web/src/foo/bar.ts', line: 12 })
    expect(c.startIndex).toBe('error in '.length)
  })

  it('matches line:col suffixes as one link', () => {
    const [c] = parseFileLinkCandidates('api/internal/core/config/config.go:44:17: unused')
    expect(c.text).toBe('api/internal/core/config/config.go:44:17')
    expect(c.line).toBe(44)
  })

  it('matches absolute and dot-relative paths', () => {
    expect(parseFileLinkCandidates('see /tmp/crowbar-daemon.log now')[0]).toMatchObject({
      path: '/tmp/crowbar-daemon.log',
    })
    expect(parseFileLinkCandidates('open ./scripts/build.sh please')[0]).toMatchObject({
      path: './scripts/build.sh',
    })
    expect(parseFileLinkCandidates('cat ../other/file.txt')[0]).toMatchObject({
      path: '../other/file.txt',
    })
  })

  it('matches quoted and parenthesised paths', () => {
    expect(parseFileLinkCandidates('failed ("web/src/main.tsx")')[0]).toMatchObject({
      path: 'web/src/main.tsx',
    })
  })

  it('does NOT match the path portion of URLs', () => {
    expect(parseFileLinkCandidates('visit https://github.com/char2cs/crowbar for info')).toEqual([])
    expect(parseFileLinkCandidates('at http://localhost:5173/some/route')).toEqual([])
  })

  it('does NOT match bare filenames or prose', () => {
    expect(parseFileLinkCandidates('run package.json scripts')).toEqual([])
    expect(parseFileLinkCandidates('plain words only here')).toEqual([])
  })

  it('matches multiple candidates on one row', () => {
    const found = parseFileLinkCandidates('diff a/web/index.ts b/web/index.ts')
    expect(found.map((c) => c.path)).toEqual(['a/web/index.ts', 'b/web/index.ts'])
  })
})

describe('resolveFileLinkPath', () => {
  const root = '/Users/dev/project'

  it('keeps absolute paths', () => {
    expect(resolveFileLinkPath('/tmp/x.log', root)).toBe('/tmp/x.log')
  })

  it('joins relative paths onto the root', () => {
    expect(resolveFileLinkPath('web/src/a.ts', root)).toBe('/Users/dev/project/web/src/a.ts')
    expect(resolveFileLinkPath('./web/a.ts', root)).toBe('/Users/dev/project/web/a.ts')
  })

  it('normalises .. segments', () => {
    expect(resolveFileLinkPath('../sibling/f.ts', root)).toBe('/Users/dev/sibling/f.ts')
  })

  it('returns null without a usable root or for ~ paths', () => {
    expect(resolveFileLinkPath('web/a.ts', undefined)).toBeNull()
    expect(resolveFileLinkPath('~/notes.md', root)).toBeNull()
  })
})

// handleFileOpen / the files API only accept worktree-RELATIVE paths, so the
// resolved absolute path must be mapped back onto the workspace root.
describe('workspaceRelativePath', () => {
  const root = '/Users/dev/project'

  it('relativizes paths inside the root', () => {
    expect(workspaceRelativePath('/Users/dev/project/web/a.ts', root)).toBe('web/a.ts')
    expect(workspaceRelativePath('/Users/dev/project/README.md', root + '/')).toBe('README.md')
  })

  it('rejects paths outside the root', () => {
    expect(workspaceRelativePath('/tmp/x.log', root)).toBeNull()
    // Prefix trap: /Users/dev/project-two is NOT inside /Users/dev/project.
    expect(workspaceRelativePath('/Users/dev/project-two/a.ts', root)).toBeNull()
  })

  it('rejects the root itself and missing roots', () => {
    expect(workspaceRelativePath('/Users/dev/project', root)).toBeNull()
    expect(workspaceRelativePath('/Users/dev/project/a.ts', undefined)).toBeNull()
  })
})
