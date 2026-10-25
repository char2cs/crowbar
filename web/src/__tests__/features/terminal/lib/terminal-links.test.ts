import { beforeEach, describe, it, expect, vi } from 'vitest'
import type { ILinkProvider, Terminal } from '@xterm/xterm'

const { revealInEditor } = vi.hoisted(() => ({ revealInEditor: vi.fn() }))
vi.mock('@/features/editor/lib/reveal', () => ({ revealInEditor }))
vi.mock('@/features/workspace/stores/workspace-store-registry', async (importOriginal) => ({
  ...(await importOriginal<object>()),
  getActiveWorkspaceId: () => 'ws-a',
}))

import {
  parseFileLinkCandidates,
  registerTerminalLinks,
  resolveFileLinkPath,
  terminalLinksFor,
} from '@/features/terminal/lib/terminal-links'
import { useSidebarStore, type Repo } from '@/lib/store/sidebar'

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

  it('strips a sentence-ending period run straight into the path', () => {
    const [c] = parseFileLinkCandidates(
      'Spec written to docs/superpowers/specs/design.md. Please review it',
    )
    expect(c).toMatchObject({
      path: 'docs/superpowers/specs/design.md',
      text: 'docs/superpowers/specs/design.md',
    })
  })

  it('keeps a line suffix when trailing punctuation follows it', () => {
    const [c] = parseFileLinkCandidates('see web/src/foo.ts:12. done')
    expect(c).toMatchObject({ path: 'web/src/foo.ts', text: 'web/src/foo.ts:12', line: 12 })
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

describe('terminalLinksFor', () => {
  beforeEach(() => {
    revealInEditor.mockReset()
    const ws = (id: string, localPath: string) => ({ id, branch: id, localPath })
    useSidebarStore.setState({
      repos: [
        {
          id: 'r1',
          defaultWorkspaceId: 'ws-main',
          localPath: '/disk/repo',
          workspaces: [ws('ws-a', '/disk/a'), ws('ws-b', '/disk/b')],
        } as unknown as Repo,
      ],
    })
  })

  /** Clicks the first file link xterm would draw on a row reading `text`. */
  function click(text: string, workspaceId: string | undefined) {
    let provider: ILinkProvider | undefined
    const terminal = {
      registerLinkProvider: (p: ILinkProvider) => ((provider = p), { dispose: () => {} }),
      buffer: { active: { getLine: () => ({ translateToString: () => text }) } },
    } as unknown as Terminal
    registerTerminalLinks(
      terminal,
      terminalLinksFor(
        () => 'tab-1',
        () => workspaceId,
      ),
    )
    provider?.provideLinks(1, (links) => links?.[0]?.activate(new MouseEvent('click'), text))
  }

  it("resolves a relative link against its own terminal's workspace, not the active one", () => {
    click('error in src/a.ts:10', 'ws-b')

    expect(revealInEditor).toHaveBeenCalledExactlyOnceWith({
      doc: { absolutePath: '/disk/b/src/a.ts' },
      position: { line: 9, character: 0 },
    })
  })

  it('a terminal of no workspace opens absolute links only', () => {
    click('error in src/a.ts:10', undefined)
    click('see /etc/hosts', undefined)

    expect(revealInEditor.mock.calls).toEqual([[{ doc: { absolutePath: '/etc/hosts' } }]])
  })
})
