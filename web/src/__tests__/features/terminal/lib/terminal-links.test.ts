import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { afterEach, beforeEach, describe, it, expect, vi } from 'vitest'
import { Terminal, type ILink, type ILinkProvider } from '@xterm/xterm'

const { revealInEditor, showTerminalUrlMenu } = vi.hoisted(() => ({
  revealInEditor: vi.fn(),
  showTerminalUrlMenu: vi.fn().mockResolvedValue(undefined),
}))
vi.mock('@/features/editor/lib/reveal', () => ({ revealInEditor }))
vi.mock('@/features/terminal/lib/terminal-url-menu', () => ({ showTerminalUrlMenu }))

import {
  registerTerminalLinks,
  terminalLinksFor,
  type TerminalLinksOptions,
} from '@/features/terminal/lib/terminal-links'
import { setActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'
import { toast } from '@/features/window/stores/toast-store'
import type { Repo } from '@/core/entities/repo-types'
import { IS_MAC } from '@/utils/platform'
import { seedRepos } from '@/__tests__/__fixtures__/sidebar-entities'

// The daemon's Go conformance suite wraps the same URL (wrapTestURL, 58 chars).
const WRAP_URL = 'https://example.com/docs/guide/getting-started?ref=crowbar'

// A link opens only on a click with the platform's primary modifier (Cmd on macOS, Ctrl elsewhere).
const PRIMARY = IS_MAC ? { metaKey: true } : { ctrlKey: true }
const OTHER = IS_MAC ? { ctrlKey: true, altKey: true } : { metaKey: true, altKey: true }
const primaryClick = (init: MouseEventInit = {}) =>
  new MouseEvent('mouseup', { clientX: 40, clientY: 50, ...PRIMARY, ...init })

const terminals: Terminal[] = []
const registrations: Array<{ dispose: () => void }> = []
afterEach(() => {
  for (const registration of registrations.splice(0)) registration.dispose()
  for (const terminal of terminals.splice(0)) terminal.dispose()
  document.body.innerHTML = ''
})

async function terminalWith(data: string, cols = 80, rows = 6): Promise<Terminal> {
  const terminal = new Terminal({ cols, rows, allowProposedApi: true })
  terminals.push(terminal)
  await new Promise<void>((resolve) => terminal.write(data, resolve))
  return terminal
}

function register(terminal: Terminal, options: TerminalLinksOptions): ILinkProvider {
  let provider: ILinkProvider | undefined
  const registerLinkProvider = (p: ILinkProvider) => ((provider = p), { dispose: () => {} })
  registrations.push(
    registerTerminalLinks(
      { buffer: terminal.buffer, element: terminal.element, registerLinkProvider },
      options,
    ),
  )
  if (!provider) throw new Error('no link provider registered')
  return provider
}

const withDefaults = (options: Partial<TerminalLinksOptions>): TerminalLinksOptions => ({
  getRoot: () => '/repo',
  openFile: vi.fn(),
  onUrl: vi.fn(),
  ...options,
})

const linkProvider = (terminal: Terminal, options: Partial<TerminalLinksOptions> = {}) =>
  register(terminal, withDefaults(options))

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
  const spies = { openFile: vi.fn(), onUrl: vi.fn(), onUnresolved: vi.fn() }
  const provider = linkProvider(await terminalWith(data, cols), { getRoot, ...spies })
  const [link] = (await linksAt(provider, row)) ?? []
  link?.activate(primaryClick(), link.text)
  return spies
}

describe('link tokens', () => {
  it.each([
    ['a URL without its sentence-ending period', 'see https://x.dev/a.', ['https://x.dev/a']],
    ['a URL without its brackets', '(https://x.dev/a)', ['https://x.dev/a']],
    ['no URL but http and https', 'ftp://x or www.x.dev', []],
    ['no URL that does not parse', 'see https://[oops', []],
    ['no URL whose numeric host the parser rewrites', 'see https://0x7f.1/a', []],
    ['no URL whose unicode host the parser rewrites', 'see https://bücher.de/a', []],
    ['an absolute path', 'see /tmp/crowbar-daemon.log now', ['/tmp/crowbar-daemon.log']],
    ['a dot-relative path', 'open ./scripts/build.sh please', ['./scripts/build.sh']],
    ['a parent-relative path', 'cat ../other/file.txt', ['../other/file.txt']],
    ['a quoted, parenthesised path', 'failed ("web/src/main.tsx")', ['web/src/main.tsx']],
    ['no bare filename', 'run package.json scripts', []],
    ['no prose', 'plain words only here', []],
    [
      'each path on a row',
      'diff a/web/index.ts b/web/index.ts',
      ['a/web/index.ts', 'b/web/index.ts'],
    ],
    ['a path without its period', 'Spec in docs/specs/design.md. Please', ['docs/specs/design.md']],
    ['a line suffix before a period', 'see web/src/foo.ts:12. done', ['web/src/foo.ts:12']],
    ['a path ending at a URL', 'see out/https://x.dev/a', ['https://x.dev/a', 'out/https']],
  ])('links %s', async (_, data, texts) => {
    expect(await textsOf(data)).toEqual(texts)
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

  it('a path in a run that starts with any scheme or www. is not a file link', async () => {
    expect(await textsOf('see ftp://h/(a/b.ts)')).toEqual([])
    expect(await textsOf('see www.x.dev/web/src/a.ts')).toEqual([])
    expect(await textsOf('www.x.dev/web/src/a.ts')).toEqual([])
    const opened = await Promise.all(
      ['(', '[', '<', '"', "'", '`'].map((o) => textsOf(`see ${o}www.x.dev/a/b.ts`)),
    )
    expect(opened).toEqual([[], [], [], [], [], []])
    expect(await textsOf('see xwww.dev/a/b.ts')).toEqual(['xwww.dev/a/b.ts'])
  })

  it('a www. inside a path never starts an address run', async () => {
    const paths = [
      'bin/www.js:12',
      './www.ts:5',
      'src/my-www.config.ts',
      'docs/www.example.com/index.html',
    ]
    const found = await Promise.all(paths.map((path) => textsOf('see ' + path)))
    expect(found).toEqual(paths.map((path) => [path]))
    expect(await textsOf('see ["bin/www.js","lib/a.ts"]')).toEqual(['bin/www.js', 'lib/a.ts'])
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

  it('reads cells the cursor skipped before content as spaces', async () => {
    const [link] =
      (await linksAt(linkProvider(await terminalWith('see\x1b[3C/tmp/a.log')), 1)) ?? []
    expect(link?.text).toBe('/tmp/a.log')
    expect(link?.range.start).toEqual({ x: 7, y: 1 })
  })
})

describe('file link resolution', () => {
  const getRoot = () => '/Users/dev/project'

  it('keeps absolute paths and joins relative ones onto the root', async () => {
    const opened = async (data: string) => (await click(data, { getRoot })).openFile.mock.calls
    expect(await opened('see /tmp/x.log')).toEqual([['/tmp/x.log', undefined]])
    expect(await opened('see web/src/a.ts')).toEqual([
      ['/Users/dev/project/web/src/a.ts', undefined],
    ])
    expect(await opened('see ./web/a.ts')).toEqual([['/Users/dev/project/web/a.ts', undefined]])
    expect(await opened('see ../sibling/f.ts')).toEqual([['/Users/dev/sibling/f.ts', undefined]])
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

  it('hands a URL to onUrl once, with the pointer position', async () => {
    const { onUrl, openFile } = await click('see ' + WRAP_URL, { row: 2, cols: 40 })
    expect(onUrl).toHaveBeenCalledExactlyOnceWith(WRAP_URL, { x: 40, y: 50 })
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

/** A terminal opened into the document, so its element carries the modifier events. */
async function openedTerminal(data: string, options: Partial<TerminalLinksOptions> = {}) {
  const terminal = await terminalWith(data, 40)
  const container = document.createElement('div')
  document.body.appendChild(container)
  terminal.open(container)
  const all = withDefaults(options)
  const provider = register(terminal, all)
  return { terminal, options: all, links: async () => (await linksAt(provider, 1)) ?? [] }
}

describe('link activation', () => {
  it('does nothing on a plain click', async () => {
    const { options, links } = await openedTerminal('https://a.test/x web/src/a.ts:3')
    for (const link of await links()) link.activate(new MouseEvent('mouseup'), link.text)
    expect([options.onUrl, options.openFile].map((fn) => vi.mocked(fn).mock.calls)).toEqual([
      [],
      [],
    ])
  })

  it("does not treat the other platform's modifier as the gate", async () => {
    const { options, links } = await openedTerminal('https://a.test/x')
    const [link] = await links()
    link.activate(new MouseEvent('mouseup', OTHER), link.text)
    expect(options.onUrl).not.toHaveBeenCalled()
  })
})

describe('link affordance', () => {
  const press = (terminal: Terminal, type: 'keydown' | 'keyup', held: boolean) =>
    terminal.element?.dispatchEvent(new KeyboardEvent(type, held ? PRIMARY : {}))

  it('shows underline and pointer only while the modifier is held over the link', async () => {
    const { terminal, links } = await openedTerminal('https://a.test/x')
    const [link] = await links()
    expect(link.decorations).toEqual({ underline: false, pointerCursor: false })

    link.hover?.(new MouseEvent('mousemove'), link.text)
    press(terminal, 'keydown', true)
    expect(link.decorations).toEqual({ underline: true, pointerCursor: true })

    press(terminal, 'keyup', false)
    expect(link.decorations).toEqual({ underline: false, pointerCursor: false })

    press(terminal, 'keydown', true)
    window.dispatchEvent(new Event('blur'))
    expect(link.decorations).toEqual({ underline: false, pointerCursor: false })
  })

  it('starts underlined when the modifier is already held as the link is provided', async () => {
    const { terminal, links } = await openedTerminal('https://a.test/x')
    terminal.element?.dispatchEvent(new MouseEvent('mousemove', PRIMARY))
    const [link] = await links()
    expect(link.decorations).toEqual({ underline: true, pointerCursor: true })
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
    showTerminalUrlMenu.mockClear()
    setActiveWorkspaceId('ws-a')
    const ws = (id: string, localPath: string) => ({ id, branch: id, localPath })
    seedRepos([
      {
        id: 'r1',
        defaultWorkspaceId: 'ws-main',
        localPath: '/disk/repo',
        workspaces: [ws('ws-a', '/disk/a'), ws('ws-b', '/disk/b')],
      } as unknown as Repo,
    ])
  })

  /** Clicks the first link xterm would draw on a row reading `text`. */
  async function clickIn(text: string, workspaceId: string | undefined) {
    const options = terminalLinksFor(
      () => 'tab-1',
      () => ({ workspaceId, paneId: workspaceId && 'pane-1' }),
    )
    const [link] = (await linksAt(register(await terminalWith(text), options), 1)) ?? []
    link?.activate(primaryClick(), link.text)
  }

  it("resolves a relative link against its own terminal's workspace, not the active one", async () => {
    await clickIn('error in src/a.ts:10', 'ws-b')

    expect(revealInEditor).toHaveBeenCalledExactlyOnceWith({
      doc: { authority: 'workspace', workspaceId: 'ws-b', path: 'src/a.ts' },
      position: { line: 9, character: 0 },
    })
  })

  it("opens a local terminal's path outside its worktree as a file of this machine", async () => {
    await clickIn('see /etc/hosts', 'ws-b')

    expect(revealInEditor.mock.calls).toEqual([[{ doc: { absolutePath: '/etc/hosts' } }]])
  })

  it('a terminal of no workspace opens absolute links only', async () => {
    await clickIn('error in src/a.ts:10', undefined)
    await clickIn('see /etc/hosts', undefined)

    expect(revealInEditor.mock.calls).toEqual([[{ doc: { absolutePath: '/etc/hosts' } }]])
  })

  it("offers a URL's menu in the terminal's own view", async () => {
    await clickIn('docs at https://x.dev/guide', 'ws-b')

    expect(showTerminalUrlMenu).toHaveBeenCalledExactlyOnceWith(
      'https://x.dev/guide',
      { x: 40, y: 50 },
      { workspaceId: 'ws-b', paneId: 'pane-1' },
    )
    expect(revealInEditor).not.toHaveBeenCalled()
  })

  it('reports a path it cannot resolve', async () => {
    const toastError = vi.spyOn(toast, 'error').mockImplementation(() => '')
    await clickIn('see ~/notes.md', 'ws-b')

    expect(toastError).toHaveBeenCalledExactlyOnceWith(
      'Cannot open file',
      'Could not resolve ~/notes.md to a path.',
    )
    toastError.mockRestore()
  })
})
