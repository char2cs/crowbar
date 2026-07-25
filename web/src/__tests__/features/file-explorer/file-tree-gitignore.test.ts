import { describe, expect, it } from 'vitest'
import type { FileEntry } from '@/features/file-system/types/app'
import {
  collectGitIgnoreFileReferences,
  createFileTreeGitIgnoreRules,
  isPathGitIgnoredByFileTreeRules,
} from '@/features/file-explorer/file-explorer/lib/file-tree-gitignore'

const dir = (name: string, path: string, children?: FileEntry[]): FileEntry => ({
  name,
  path,
  isDir: true,
  children,
})

const file = (name: string, path: string): FileEntry => ({
  name,
  path,
  isDir: false,
})

describe('file tree gitignore rules', () => {
  it('collects root and nested .gitignore files from the loaded tree', () => {
    const references = collectGitIgnoreFileReferences(
      [
        file('.gitignore', '/repo/.gitignore'),
        dir('subprojects', '/repo/subprojects', [
          file('.gitignore', '/repo/subprojects/.gitignore'),
          dir('nested', '/repo/subprojects/nested', [
            file('.gitignore', '/repo/subprojects/nested/.gitignore'),
          ]),
        ]),
        file('.gitignore', '/other/.gitignore'),
      ],
      '/repo',
    )

    expect(references.map((reference) => reference.path)).toEqual([
      '/repo/.gitignore',
      '/repo/subprojects/.gitignore',
      '/repo/subprojects/nested/.gitignore',
    ])
  })

  // Regression: never synthesize a conventional root .gitignore reference. The
  // backend tree includes dotfiles, so a real root .gitignore is surfaced by the
  // walk; synthesizing one when the project root has none (common for the
  // project-home workspace) fetched a non-existent file and 404'd on every load.
  it('does not synthesize a root .gitignore reference when the loaded tree has none', () => {
    const references = collectGitIgnoreFileReferences(
      [dir('crowbar', 'crowbar'), dir('desktop', 'desktop')],
      '/repo',
    )
    expect(references).toEqual([])
  })

  it('returns no references for an empty (not-yet-loaded) tree instead of synthesizing one', () => {
    expect(collectGitIgnoreFileReferences([], '/repo')).toEqual([])
  })

  it('applies nested .gitignore files relative to the directory that owns them', () => {
    const rules = createFileTreeGitIgnoreRules('/repo', [
      {
        path: '/repo/.gitignore',
        directoryPath: '/repo',
        content: 'target/\n*.tmp\n',
      },
      {
        path: '/repo/subprojects/.gitignore',
        directoryPath: '/repo/subprojects',
        content: '/**/\npackagecache\nchumsky\n',
      },
    ])

    expect(isPathGitIgnoredByFileTreeRules(rules, '/repo/target', true)).toBe(true)
    expect(isPathGitIgnoredByFileTreeRules(rules, '/repo/src/file.tmp', false)).toBe(true)
    expect(isPathGitIgnoredByFileTreeRules(rules, '/repo/subprojects/zerocopy-0.8.48', true)).toBe(
      true,
    )
    expect(isPathGitIgnoredByFileTreeRules(rules, '/repo/subprojects/packagecache', true)).toBe(
      true,
    )
    expect(isPathGitIgnoredByFileTreeRules(rules, '/repo/zerocopy-0.8.48', true)).toBe(false)
    expect(isPathGitIgnoredByFileTreeRules(rules, '/repo/src/main.ts', false)).toBe(false)
  })

  it('lets lower .gitignore files unignore files ignored by parent rules', () => {
    const rules = createFileTreeGitIgnoreRules('/repo', [
      {
        path: '/repo/.gitignore',
        directoryPath: '/repo',
        content: '*.log\n',
      },
      {
        path: '/repo/logs/.gitignore',
        directoryPath: '/repo/logs',
        content: '!keep.log\n',
      },
    ])

    expect(isPathGitIgnoredByFileTreeRules(rules, '/repo/logs/error.log', false)).toBe(true)
    expect(isPathGitIgnoredByFileTreeRules(rules, '/repo/logs/keep.log', false)).toBe(false)
  })

  it('keeps files ignored when an ancestor directory is ignored', () => {
    const rules = createFileTreeGitIgnoreRules('/repo', [
      {
        path: '/repo/.gitignore',
        directoryPath: '/repo',
        content: 'logs/\n',
      },
      {
        path: '/repo/logs/.gitignore',
        directoryPath: '/repo/logs',
        content: '!keep.log\n',
      },
    ])

    expect(isPathGitIgnoredByFileTreeRules(rules, '/repo/logs', true)).toBe(true)
    expect(isPathGitIgnoredByFileTreeRules(rules, '/repo/logs/keep.log', false)).toBe(true)
  })

  it('handles anchored nested patterns and directory-only patterns', () => {
    const rules = createFileTreeGitIgnoreRules('/repo', [
      {
        path: '/repo/sub/.gitignore',
        directoryPath: '/repo/sub',
        content: '/cache\nbuild/\n',
      },
    ])

    expect(isPathGitIgnoredByFileTreeRules(rules, '/repo/sub/cache', false)).toBe(true)
    expect(isPathGitIgnoredByFileTreeRules(rules, '/repo/sub/deep/cache', false)).toBe(false)
    expect(isPathGitIgnoredByFileTreeRules(rules, '/repo/sub/build', true)).toBe(true)
    expect(isPathGitIgnoredByFileTreeRules(rules, '/repo/sub/build.txt', false)).toBe(false)
  })

  it('supports Windows paths after normalizing matcher input', () => {
    const rules = createFileTreeGitIgnoreRules('C:\\repo', [
      {
        path: 'C:\\repo\\sub\\.gitignore',
        directoryPath: 'C:\\repo\\sub',
        content: '*.gen.ts\n',
      },
    ])

    expect(isPathGitIgnoredByFileTreeRules(rules, 'C:\\repo\\sub\\client.gen.ts', false)).toBe(true)
    expect(isPathGitIgnoredByFileTreeRules(rules, 'C:\\repo\\other\\client.gen.ts', false)).toBe(
      false,
    )
  })

  it('keeps valid rules when a .gitignore contains a malformed line', () => {
    const rules = createFileTreeGitIgnoreRules('/repo', [
      {
        path: '/repo/.gitignore',
        directoryPath: '/repo',
        content: 'valid.out\nbad\\\n\\#literal-hash\n',
      },
    ])

    expect(isPathGitIgnoredByFileTreeRules(rules, '/repo/valid.out', false)).toBe(true)
    expect(isPathGitIgnoredByFileTreeRules(rules, '/repo/#literal-hash', false)).toBe(true)
    expect(isPathGitIgnoredByFileTreeRules(rules, '/other/valid.out', false)).toBe(false)
  })

  describe('workspace-relative paths (synthetic root)', () => {
    const rootFolderPath = '/repos/repo-1'

    it('collects workspace-relative .gitignore files under a synthetic root', () => {
      const references = collectGitIgnoreFileReferences(
        [
          file('.gitignore', '.gitignore'),
          dir('web', 'web', [file('.gitignore', 'web/.gitignore')]),
          file('.gitignore', '/other/.gitignore'),
        ],
        rootFolderPath,
      )

      expect(references.map((reference) => reference.path)).toEqual([
        '.gitignore',
        'web/.gitignore',
      ])
      expect(references[0]?.directoryPath).toBe('')
    })

    it.each(['', '.'])(
      'matches relative tree paths against a root .gitignore with directoryPath %j',
      (directoryPath) => {
        const rules = createFileTreeGitIgnoreRules(rootFolderPath, [
          {
            path: '.gitignore',
            directoryPath,
            content: 'node_modules\n',
          },
        ])

        expect(isPathGitIgnoredByFileTreeRules(rules, 'node_modules', true)).toBe(true)
        expect(isPathGitIgnoredByFileTreeRules(rules, 'node_modules/x', false)).toBe(true)
        expect(isPathGitIgnoredByFileTreeRules(rules, 'web', true)).toBe(false)
        expect(isPathGitIgnoredByFileTreeRules(rules, 'web/src/x.ts', false)).toBe(false)
      },
    )

    it('keeps relative paths ignored when an ignored ancestor directory contains them', () => {
      const rules = createFileTreeGitIgnoreRules(rootFolderPath, [
        {
          path: '.gitignore',
          directoryPath: '',
          content: 'dist/\n',
        },
        {
          path: 'dist/.gitignore',
          directoryPath: 'dist',
          content: '!keep.txt\n',
        },
      ])

      expect(isPathGitIgnoredByFileTreeRules(rules, 'dist', true)).toBe(true)
      expect(isPathGitIgnoredByFileTreeRules(rules, 'dist/keep.txt', false)).toBe(true)
    })

    it('applies nested workspace-relative .gitignore files to their own directory', () => {
      const rules = createFileTreeGitIgnoreRules(rootFolderPath, [
        {
          path: 'web/.gitignore',
          directoryPath: 'web',
          content: '*.gen.ts\n',
        },
      ])

      expect(isPathGitIgnoredByFileTreeRules(rules, 'web/client.gen.ts', false)).toBe(true)
      expect(isPathGitIgnoredByFileTreeRules(rules, 'api/client.gen.ts', false)).toBe(false)
    })

    it('lets nested relative .gitignore files unignore parent rules', () => {
      const rules = createFileTreeGitIgnoreRules(rootFolderPath, [
        {
          path: '.gitignore',
          directoryPath: '',
          content: '*.log\n',
        },
        {
          path: 'logs/.gitignore',
          directoryPath: 'logs',
          content: '!keep.log\n',
        },
      ])

      expect(isPathGitIgnoredByFileTreeRules(rules, 'logs/error.log', false)).toBe(true)
      expect(isPathGitIgnoredByFileTreeRules(rules, 'logs/keep.log', false)).toBe(false)
    })

    it('does not treat the relative .git directory as ignored', () => {
      const rules = createFileTreeGitIgnoreRules(rootFolderPath, [
        {
          path: '.gitignore',
          directoryPath: '',
          content: '.git\n*\n',
        },
      ])

      expect(isPathGitIgnoredByFileTreeRules(rules, '.git', true)).toBe(false)
      expect(isPathGitIgnoredByFileTreeRules(rules, 'file.txt', false)).toBe(true)
    })

    it('does not match absolute paths against a root-relative rule set', () => {
      const rules = createFileTreeGitIgnoreRules(rootFolderPath, [
        {
          path: '.gitignore',
          directoryPath: '',
          content: 'node_modules\n',
        },
      ])

      expect(isPathGitIgnoredByFileTreeRules(rules, '/other/node_modules', true)).toBe(false)
    })
  })

  it('does not treat the repository root or .git directory as ignored', () => {
    const rules = createFileTreeGitIgnoreRules('/repo', [
      {
        path: '/repo/.gitignore',
        directoryPath: '/repo',
        content: '.git\n*\n',
      },
    ])

    expect(isPathGitIgnoredByFileTreeRules(rules, '/repo', true)).toBe(false)
    expect(isPathGitIgnoredByFileTreeRules(rules, '/repo/.git', true)).toBe(false)
    expect(isPathGitIgnoredByFileTreeRules(rules, '/repo/file.txt', false)).toBe(true)
  })
})
