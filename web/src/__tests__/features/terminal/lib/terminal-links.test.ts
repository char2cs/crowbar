import { afterEach, describe, expect, it, vi } from 'vitest'
import { Terminal, type ILink, type ILinkProvider } from '@xterm/xterm'
import {
  registerTerminalLinks,
  type TerminalLinksOptions,
} from '@/features/terminal/lib/terminal-links'

// The gestures below are macOS's (Cmd); pin the platform so Linux CI runs them too.
vi.mock('@/utils/platform', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/utils/platform')>()),
  IS_MAC: true,
  APPLE_KEYBOARD: true,
}))

const cleanup: Array<() => void> = []
afterEach(() => {
  for (const fn of cleanup.splice(0)) fn()
  document.body.innerHTML = ''
})

function write(terminal: Terminal, data: string): Promise<void> {
  return new Promise((resolve) => terminal.write(data, resolve))
}

// The daemon repaints rows by absolute address, so xterm never sees a wrap.
const repaint = (rows: string[]) => rows.map((r, i) => `\x1b[${i + 1};1H${r}`).join('')

function setup(cols = 20, overrides: Partial<TerminalLinksOptions> = {}) {
  const container = document.createElement('div')
  document.body.appendChild(container)
  const terminal = new Terminal({ cols, rows: 8, allowProposedApi: true })
  terminal.open(container)
  let provider: ILinkProvider | undefined
  const register = terminal.registerLinkProvider.bind(terminal)
  terminal.registerLinkProvider = (p) => {
    provider = p
    return register(p)
  }
  const options: TerminalLinksOptions = {
    getRoot: () => '/repo',
    openFile: vi.fn(),
    onUrl: vi.fn(),
    ...overrides,
  }
  const links = registerTerminalLinks(terminal, options)
  cleanup.push(
    () => links.dispose(),
    () => terminal.dispose(),
  )
  const linksAt = (row: number) =>
    new Promise<ILink[]>((resolve) => {
      provider?.provideLinks(row, (found) => resolve(found ?? []))
    })
  return { terminal, options, linksAt }
}

const click = (init: MouseEventInit = {}) =>
  new MouseEvent('mouseup', { clientX: 40, clientY: 50, ...init })

describe('registerTerminalLinks detection', () => {
  const url = 'https://example.com/a/very/long/path/that/wraps/twice'

  it('detects a URL repainted across three rows as one link spanning all of them', async () => {
    const { terminal, linksAt } = setup(20)
    await write(terminal, repaint([url.slice(0, 20), url.slice(20, 40), url.slice(40)]))

    for (const row of [1, 2, 3]) {
      const [link] = await linksAt(row)
      expect(link.text).toBe(url)
      expect(link.range).toEqual({
        start: { x: 1, y: 1 },
        end: { x: url.length - 40, y: 3 },
      })
    }
  })

  it('detects a soft-wrapped URL the same way', async () => {
    const { terminal, linksAt } = setup(20)
    await write(terminal, url)

    const [link] = await linksAt(2)
    expect(link.text).toBe(url)
    expect(link.range.start).toEqual({ x: 1, y: 1 })
    expect(link.range.end.y).toBe(3)
  })

  it('ends exactly at the row boundary without spilling onto the next row', async () => {
    const exact = 'https://example.com/12' // 22 chars -> use 2 rows of 11
    const { terminal, linksAt } = setup(11)
    await write(terminal, repaint([exact.slice(0, 11), exact.slice(11)]))

    const [link] = await linksAt(2)
    expect(link.text).toBe(exact)
    expect(link.range.end).toEqual({ x: 11, y: 2 })
  })

  it('trims trailing punctuation like the web-links addon', async () => {
    const { terminal, linksAt } = setup(20)
    await write(terminal, repaint(['see https://example.', 'com/docs). done']))

    const [link] = await linksAt(1)
    expect(link.text).toBe('https://example.com/docs')
    expect(link.range.end).toEqual({ x: 8, y: 2 })
  })

  it('does not join rows that ended early', async () => {
    const { terminal, linksAt } = setup(20)
    await write(terminal, repaint(['https://a.test/x', 'continued/not/a/link']))

    const [link] = await linksAt(1)
    expect(link.text).toBe('https://a.test/x')
    expect(link.range.end.y).toBe(1)
  })

  it('detects a path:line link across a wrapped row', async () => {
    const { terminal, linksAt } = setup(20)
    await write(terminal, repaint(['see web/src/feature/', 'deep/file.ts:12 now']))

    const [link] = await linksAt(2)
    expect(link.text).toBe('web/src/feature/deep/file.ts:12')
    expect(link.range.start).toEqual({ x: 5, y: 1 })
    expect(link.range.end).toEqual({ x: 15, y: 2 })
  })
})

describe('registerTerminalLinks activation', () => {
  it('does nothing on a plain click', async () => {
    const { terminal, options, linksAt } = setup(40)
    await write(terminal, repaint(['https://a.test/x web/src/a.ts:3']))

    const [url, file] = await linksAt(1)
    url.activate(click(), url.text)
    file.activate(click(), file.text)

    expect(options.onUrl).not.toHaveBeenCalled()
    expect(options.openFile).not.toHaveBeenCalled()
  })

  it('hands a modifier-click on a URL to onUrl with the pointer position', async () => {
    const { terminal, options, linksAt } = setup(40)
    await write(terminal, repaint(['https://a.test/x']))

    const [link] = await linksAt(1)
    link.activate(click({ metaKey: true }), link.text)

    expect(options.onUrl).toHaveBeenCalledExactlyOnceWith('https://a.test/x', { x: 40, y: 50 })
  })

  it('does not treat the other platform modifier as the gate', async () => {
    const { terminal, options, linksAt } = setup(40)
    await write(terminal, repaint(['https://a.test/x']))

    const [link] = await linksAt(1)
    link.activate(click({ ctrlKey: true, altKey: true }), link.text)

    expect(options.onUrl).not.toHaveBeenCalled()
  })

  it('opens a file directly on a modifier-click', async () => {
    const { terminal, options, linksAt } = setup(40)
    await write(terminal, repaint(['at web/src/a.ts:3']))

    const [link] = await linksAt(1)
    link.activate(click({ metaKey: true }), link.text)

    expect(options.openFile).toHaveBeenCalledExactlyOnceWith('/repo/web/src/a.ts', 3)
  })

  it('reports an unresolvable file on a modifier-click', async () => {
    const onUnresolved = vi.fn()
    const { terminal, linksAt } = setup(40, { getRoot: () => undefined, onUnresolved })
    await write(terminal, repaint(['at web/src/a.ts:3']))

    const [link] = await linksAt(1)
    link.activate(click({ metaKey: true }), link.text)

    expect(onUnresolved).toHaveBeenCalledExactlyOnceWith('web/src/a.ts:3')
  })
})

describe('registerTerminalLinks affordance', () => {
  it('shows underline and pointer only while the modifier is held over the link', async () => {
    const { terminal, linksAt } = setup(40)
    await write(terminal, repaint(['https://a.test/x']))
    const [link] = await linksAt(1)
    expect(link.decorations).toEqual({ underline: false, pointerCursor: false })

    link.hover?.(new MouseEvent('mousemove'), link.text)
    terminal.element?.dispatchEvent(new KeyboardEvent('keydown', { key: 'Meta', metaKey: true }))
    expect(link.decorations).toEqual({ underline: true, pointerCursor: true })

    terminal.element?.dispatchEvent(new KeyboardEvent('keyup', { key: 'Meta' }))
    expect(link.decorations).toEqual({ underline: false, pointerCursor: false })

    terminal.element?.dispatchEvent(new KeyboardEvent('keydown', { key: 'Meta', metaKey: true }))
    window.dispatchEvent(new Event('blur'))
    expect(link.decorations).toEqual({ underline: false, pointerCursor: false })
  })

  it('starts underlined when the modifier is already held as the link is provided', async () => {
    const { terminal, linksAt } = setup(40)
    await write(terminal, repaint(['https://a.test/x']))
    terminal.element?.dispatchEvent(new MouseEvent('mousemove', { metaKey: true }))

    const [link] = await linksAt(1)
    expect(link.decorations).toEqual({ underline: true, pointerCursor: true })
  })
})
