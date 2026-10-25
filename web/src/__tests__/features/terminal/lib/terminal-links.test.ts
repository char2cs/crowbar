import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { afterEach, beforeEach, describe, it, expect, vi } from 'vitest'
import { Terminal, type ILink, type ILinkProvider } from '@xterm/xterm'

const { revealInEditor, openExternalUrl } = vi.hoisted(() => ({
  revealInEditor: vi.fn(),
  openExternalUrl: vi.fn().mockResolvedValue(undefined),
}))
vi.mock('@/features/editor/lib/reveal', () => ({ revealInEditor }))
vi.mock('@/lib/external-open', () => ({ openExternalUrl }))
vi.mock('@/features/workspace/stores/workspace-store-registry', async (importOriginal) => ({
  ...(await importOriginal<object>()),
  getActiveWorkspaceId: () => 'ws-a',
}))

import {
  registerTerminalLinks,
  terminalLinksFor,
  type TerminalLinksOptions,
} from '@/features/terminal/lib/terminal-links'
import { useSidebarStore, type Repo } from '@/lib/store/sidebar'

// The daemon's Go conformance suite wraps the same URL (wrapTestURL, 58 chars).
const WRAP_URL = 'https://example.com/docs/guide/getting-started?ref=crowbar'

const terminals: Terminal[] = []
afterEach(() => {
  for (const terminal of terminals.splice(0)) terminal.dispose()
})

async function terminalWith(data: string, cols = 80, rows = 6): Promise<Terminal> {
  const terminal = new Terminal({ cols, rows, allowProposedApi: true })
  terminals.push(terminal)
  await new Promise<void>((resolve) => terminal.write(data, resolve))
  return terminal
}

function linkProvider(terminal: Terminal, options: Partial<TerminalLinksOptions> = {}) {
  let provider: ILinkProvider | undefined
  const registerLinkProvider = (p: ILinkProvider) => ((provider = p), { dispose: () => {} })
  registerTerminalLinks(
    { buffer: terminal.buffer, registerLinkProvider },
    { getRoot: () => '/repo', openFile: vi.fn(), openUrl: vi.fn(), ...options },
  )
  if (!provider) throw new Error('no link provider registered')
  return provider
}

const linksAt = (provider: ILinkProvider, row: number) =>
  new Promise<ILink[] | undefined>((resolve) => provider.provideLinks(row, resolve))

/** The link texts xterm would draw on the first row of a terminal reading `data`. */
async function textsOf(data: string): Promise<string[]> {
  const links = await linksAt(linkProvider(await terminalWith(data)), 1)
  return links?.map((link) => link.text) ?? []
}

/** Clicks the first link on row `row` and reports what it opened. */
async function click(
  data: string,
  { row = 1, cols = 80, getRoot = () => '/repo' as string | undefined },
) {
  const openFile = vi.fn()
  const openUrl = vi.fn()
  const onUnresolved = vi.fn()
  const provider = linkProvider(await terminalWith(data, cols), {
    getRoot,
    openFile,
    openUrl,
    onUnresolved,
  })
  const [link] = (await linksAt(provider, row)) ?? []
  link?.activate(new MouseEvent('click'), link.text)
  return { openFile, openUrl, onUnresolved }
}

describe('link tokens', () => {
  it('links a URL without its sentence-ending period or brackets', async () => {
    expect(await textsOf('see https://x.dev/a.')).toEqual(['https://x.dev/a'])
    expect(await textsOf('(https://x.dev/a)')).toEqual(['https://x.dev/a'])
  })

  it('links only http and https URLs that parse', async () => {
    expect(await textsOf('ftp://x or www.x.dev')).toEqual([])
    expect(await textsOf('see https://[oops')).toEqual([])
  })

  it('does not link a URL whose host the parser rewrites', async () => {
    expect(await textsOf('see https://0x7f.1/a')).toEqual([])
    expect(await textsOf('see https://bücher.de/a')).toEqual([])
  })

  it('a path inside a URL is part of the URL, never a file link of its own', async () => {
    expect(await textsOf('open https://github.com/a/b/web/src/a.ts:12')).toEqual([
      'https://github.com/a/b/web/src/a.ts:12',
    ])
    expect(await textsOf('see https://x.dev/q[web/src/a.ts:12]')).toEqual([
      'https://x.dev/q[web/src/a.ts:12',
    ])
    expect(await textsOf('visit https://github.com/char2cs/crowbar for info')).toEqual([
      'https://github.com/char2cs/crowbar',
    ])
    expect(await textsOf('at http://localhost:5173/some/route')).toEqual([
      'http://localhost:5173/some/route',
    ])
  })

  it('a path in the rest of the run a URL starts is not a file link', async () => {
    expect(await textsOf('see https://x.dev/s?q=(web/src/a.ts)')).toEqual(['https://x.dev/s?q='])
    expect(await textsOf('see https://x.dev/a web/src/b.ts')).toEqual([
      'https://x.dev/a',
      'web/src/b.ts',
    ])
  })

  it('links a repo-relative path with its line suffix', async () => {
    const [link] =
      (await linksAt(linkProvider(await terminalWith('error in web/src/foo/bar.ts:12 x')), 1)) ?? []
    expect(link.text).toBe('web/src/foo/bar.ts:12')
    expect(link.range.start).toEqual({ x: 'error in '.length + 1, y: 1 })
    const { openFile } = await click('error in web/src/foo/bar.ts:12 x', {})
    expect(openFile).toHaveBeenCalledExactlyOnceWith('/repo/web/src/foo/bar.ts', 12)
  })

  it('links a line:col suffix as one link and opens at the line', async () => {
    const data = 'api/internal/core/config/config.go:44:17: unused'
    expect(await textsOf(data)).toEqual(['api/internal/core/config/config.go:44:17'])
    expect((await click(data, {})).openFile).toHaveBeenCalledWith(
      '/repo/api/internal/core/config/config.go',
      44,
    )
  })

  it('links absolute, dot-relative, quoted and parenthesised paths', async () => {
    expect(await textsOf('see /tmp/crowbar-daemon.log now')).toEqual(['/tmp/crowbar-daemon.log'])
    expect(await textsOf('open ./scripts/build.sh please')).toEqual(['./scripts/build.sh'])
    expect(await textsOf('cat ../other/file.txt')).toEqual(['../other/file.txt'])
    expect(await textsOf('failed ("web/src/main.tsx")')).toEqual(['web/src/main.tsx'])
  })

  it('reads cells the cursor skipped before content as spaces', async () => {
    const [link] =
      (await linksAt(linkProvider(await terminalWith('see\x1b[3C/tmp/a.log')), 1)) ?? []
    expect(link?.text).toBe('/tmp/a.log')
    expect(link?.range.start).toEqual({ x: 7, y: 1 })
  })

  it('does not link bare filenames or prose', async () => {
    expect(await textsOf('run package.json scripts')).toEqual([])
    expect(await textsOf('plain words only here')).toEqual([])
  })

  it('links every path on a row', async () => {
    expect(await textsOf('diff a/web/index.ts b/web/index.ts')).toEqual([
      'a/web/index.ts',
      'b/web/index.ts',
    ])
  })

  it('drops a sentence-ending period but keeps a line suffix before it', async () => {
    expect(await textsOf('Spec in docs/specs/design.md. Please review it')).toEqual([
      'docs/specs/design.md',
    ])
    expect(await textsOf('see web/src/foo.ts:12. done')).toEqual(['web/src/foo.ts:12'])
  })
})

describe('file link resolution', () => {
  const getRoot = () => '/Users/dev/project'

  it('keeps absolute paths and joins relative ones onto the root', async () => {
    expect((await click('see /tmp/x.log', { getRoot })).openFile).toHaveBeenCalledWith(
      '/tmp/x.log',
      undefined,
    )
    expect((await click('see web/src/a.ts', { getRoot })).openFile).toHaveBeenCalledWith(
      '/Users/dev/project/web/src/a.ts',
      undefined,
    )
    expect((await click('see ./web/a.ts', { getRoot })).openFile).toHaveBeenCalledWith(
      '/Users/dev/project/web/a.ts',
      undefined,
    )
    expect((await click('see ../sibling/f.ts', { getRoot })).openFile).toHaveBeenCalledWith(
      '/Users/dev/sibling/f.ts',
      undefined,
    )
  })

  it('reports a path it cannot resolve instead of opening a wrong file', async () => {
    const noRoot = await click('see web/a.ts', { getRoot: () => undefined })
    const home = await click('see ~/notes.md', { getRoot })
    expect(noRoot.onUnresolved).toHaveBeenCalledExactlyOnceWith('web/a.ts')
    expect(home.onUnresolved).toHaveBeenCalledExactlyOnceWith('~/notes.md')
    expect([...noRoot.openFile.mock.calls, ...home.openFile.mock.calls]).toEqual([])
  })
})

describe('links over the logical line', () => {
  it('joins a URL the pane width wrapped', async () => {
    const provider = linkProvider(await terminalWith('see ' + WRAP_URL, 40))
    const range = { start: { x: 5, y: 1 }, end: { x: 22, y: 2 } }
    for (const row of [2, 1]) {
      const links = await linksAt(provider, row)
      expect(links?.map(({ text, range }) => ({ text, range }))).toEqual([
        { text: WRAP_URL, range },
      ])
    }
  })

  it('does not join a program hard-break at the last column', async () => {
    const data = 'see ' + WRAP_URL.slice(0, 36) + '\r\n' + WRAP_URL.slice(36)
    const provider = linkProvider(await terminalWith(data, 40))
    const links = await linksAt(provider, 1)
    expect(links?.map(({ text, range }) => ({ text, range }))).toEqual([
      { text: WRAP_URL.slice(0, 36), range: { start: { x: 5, y: 1 }, end: { x: 40, y: 1 } } },
    ])
    expect(await linksAt(provider, 2)).toBeUndefined()
  })

  it('returns the same link from every row of a three-row chain', async () => {
    const url = 'https://example.com/' + 'a'.repeat(80)
    const provider = linkProvider(await terminalWith('see ' + url + '\r\n\r\nplain', 40))
    const range = { start: { x: 5, y: 1 }, end: { x: 24, y: 3 } }
    for (const row of [1, 2, 3]) {
      const links = await linksAt(provider, row)
      expect(links?.map(({ text, range }) => ({ text, range }))).toEqual([{ text: url, range }])
    }
    expect(await linksAt(provider, 5)).toBeUndefined()
  })

  it('returns only the links on the hovered row', async () => {
    const data = 'see /tmp/a.log, then wrap onto the next row /tmp/b.log'
    expect(
      (await linksAt(linkProvider(await terminalWith(data, 40)), 2))?.map((l) => l.text),
    ).toEqual(['/tmp/b.log'])
  })

  it('opens a wrapped file path at its line', async () => {
    const data = 'web/src/features/terminal/lib/terminal-links.ts:12'
    const { openFile } = await click(data, { row: 2, cols: 30 })
    expect(openFile).toHaveBeenCalledExactlyOnceWith(
      '/repo/web/src/features/terminal/lib/terminal-links.ts',
      12,
    )
  })

  it('maps a link after wide glyphs onto the cells it covers', async () => {
    const provider = linkProvider(await terminalWith('日本語 ' + WRAP_URL, 40))
    const [link] = (await linksAt(provider, 1)) ?? []
    expect(link.text).toBe(WRAP_URL)
    expect(link.range).toEqual({ start: { x: 8, y: 1 }, end: { x: 25, y: 2 } })
  })

  it('joins a URL across the cell a wide glyph leaves empty at the wrap', async () => {
    const url = 'https://x.dev/' + 'a'.repeat(21) + '日本'
    const [link] = (await linksAt(linkProvider(await terminalWith('see ' + url, 40)), 2)) ?? []
    expect(link?.text).toBe(url)
    expect(link?.range).toEqual({ start: { x: 5, y: 1 }, end: { x: 4, y: 2 } })
  })

  it('opens a URL through openUrl once', async () => {
    const { openUrl, openFile } = await click('see ' + WRAP_URL, { row: 2, cols: 40 })
    expect(openUrl).toHaveBeenCalledExactlyOnceWith(WRAP_URL)
    expect(openFile).not.toHaveBeenCalled()
  })

  it('reads a bounded window of a chain far longer than 2048 characters', async () => {
    const terminal = await terminalWith('https://x.dev/' + 'a'.repeat(12_000), 40)
    const chainRows = terminal.buffer.active.baseY + terminal.buffer.active.cursorY + 1
    const getLine = vi.spyOn(terminal.buffer.active, 'getLine')
    await linksAt(linkProvider(terminal), chainRows)
    expect(chainRows).toBeGreaterThan(300)
    expect(getLine.mock.calls.length).toBeLessThan(chainRows / 3)
  })
})

// Bytes the daemon's emitter wrote, regenerated by its Go conformance suite
// (TestConformance_XtermReplayFixture) whenever the wire changes.
const fixture = JSON.parse(
  readFileSync(
    join(
      dirname(fileURLToPath(import.meta.url)),
      '../../../../../../api/internal/core/terminal/internal/model/testdata/xterm-wrapped-links.json',
    ),
    'utf8',
  ),
) as {
  cols: number
  rows: number
  scenarios: Array<{ name: string; live: string[]; keyframe: string }>
}

describe('links over bytes the daemon wrote', () => {
  const joined = {
    softWrap: true,
    hardBreak: false,
    headInScrollback: true,
    chainInScrollback: true,
  }

  it('replays every scenario the daemon generated', () => {
    expect(fixture.scenarios.map((s) => s.name)).toEqual(Object.keys(joined))
  })

  for (const { name, live, keyframe } of fixture.scenarios) {
    for (const [path, frames] of [
      ['live frames', live],
      ['re-attach keyframe', [keyframe]],
    ] as const) {
      it(`${name}, from the ${path}`, async () => {
        const terminal = await terminalWith(frames.join(''), fixture.cols, fixture.rows)
        const buffer = terminal.buffer.active
        const rows = Array.from({ length: buffer.length }, (_, y) => buffer.getLine(y))
        const head = rows.findIndex((row) => row?.translateToString().startsWith('see '))
        expect(head).toBeGreaterThanOrEqual(0)
        const provider = linkProvider(terminal)
        const texts = async (row: number) => (await linksAt(provider, row))?.map((l) => l.text)
        if (joined[name as keyof typeof joined]) {
          expect([await texts(head + 1), await texts(head + 2)]).toEqual([[WRAP_URL], [WRAP_URL]])
        } else {
          expect([await texts(head + 1), await texts(head + 2)]).toEqual([
            [WRAP_URL.slice(0, 36)],
            undefined,
          ])
        }
      })
    }
  }
})

// Pins of an external contract: the daemon's emitter relies on these xterm
// behaviours to make `isWrapped` mean "soft-wrapped" on the client.
describe('xterm wrap idioms the daemon emitter relies on', () => {
  const overflow = 'x'.repeat(45)

  it('an unbroken write past the last column flags the next row', async () => {
    expect((await terminalWith(overflow, 40)).buffer.active.getLine(1)?.isWrapped).toBe(true)
  })

  it('CUP to column 1 then EL0 clears the flag', async () => {
    const terminal = await terminalWith(overflow + '\x1b[2;1H\x1b[K', 40)
    expect(terminal.buffer.active.getLine(1)?.isWrapped).toBe(false)
  })

  it('CUP to column 1 then cells with no EL keeps the flag', async () => {
    const terminal = await terminalWith(overflow + '\x1b[2;1Hyy', 40)
    expect(terminal.buffer.active.getLine(1)?.isWrapped).toBe(true)
  })

  it('a flagged row keeps its flag in scrollback', async () => {
    const terminal = await terminalWith(overflow + '\r\n'.repeat(8), 40)
    expect(terminal.buffer.active.baseY).toBeGreaterThan(1)
    expect(terminal.buffer.active.getLine(1)?.isWrapped).toBe(true)
  })
})

describe('terminalLinksFor', () => {
  beforeEach(() => {
    revealInEditor.mockReset()
    openExternalUrl.mockClear()
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

  /** Clicks the first link xterm would draw on a row reading `text`. */
  async function clickIn(text: string, workspaceId: string | undefined) {
    const terminal = await terminalWith(text)
    let provider: ILinkProvider | undefined
    registerTerminalLinks(
      { buffer: terminal.buffer, registerLinkProvider: (p) => ((provider = p), { dispose() {} }) },
      terminalLinksFor(
        () => 'tab-1',
        () => workspaceId,
      ),
    )
    const [link] = (provider && (await linksAt(provider, 1))) || []
    link?.activate(new MouseEvent('click'), link.text)
  }

  it("resolves a relative link against its own terminal's workspace, not the active one", async () => {
    await clickIn('error in src/a.ts:10', 'ws-b')

    expect(revealInEditor).toHaveBeenCalledExactlyOnceWith({
      doc: { absolutePath: '/disk/b/src/a.ts' },
      position: { line: 9, character: 0 },
    })
  })

  it('a terminal of no workspace opens absolute links only', async () => {
    await clickIn('error in src/a.ts:10', undefined)
    await clickIn('see /etc/hosts', undefined)

    expect(revealInEditor.mock.calls).toEqual([[{ doc: { absolutePath: '/etc/hosts' } }]])
  })

  it('opens a URL in the default browser', async () => {
    await clickIn('docs at https://x.dev/guide', 'ws-b')

    expect(openExternalUrl).toHaveBeenCalledExactlyOnceWith('https://x.dev/guide')
    expect(revealInEditor).not.toHaveBeenCalled()
  })
})
