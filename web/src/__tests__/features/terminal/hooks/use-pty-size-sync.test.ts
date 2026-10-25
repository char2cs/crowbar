import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { afterEach, describe, it, expect, vi } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import { Terminal } from '@xterm/xterm'
import { usePtySizeSync } from '@/features/terminal/hooks/use-pty-size-sync'

type SyncOptions = Parameters<typeof usePtySizeSync>[0]

// usePtySizeSync is the ONE owner of a view's PTY size. These pin what the old
// refitAndSyncPty helper pinned, now at the owner: the PTY is pushed whenever
// xterm's grid differs from what THIS connection was last told — even when a fit
// changes nothing — and never redundantly, and only by a visible view.

function makeTerminal(rows: number, cols: number) {
  let onResize: (() => void) | null = null
  return {
    rows,
    cols,
    onResize: vi.fn((cb: () => void) => {
      onResize = cb
      return { dispose: () => {} }
    }),
    fireResize(nextRows: number, nextCols: number) {
      this.rows = nextRows
      this.cols = nextCols
      onResize?.()
    },
  }
}

function makeConnection() {
  return { alive: true, resize: vi.fn() }
}

// jsdom gives every element a 0×0 box; the tests control it.
function makeContainer(width = 800, height = 600) {
  const el = document.createElement('div')
  el.getBoundingClientRect = () => ({ width, height }) as DOMRect
  return el
}

function render(props: {
  terminal: ReturnType<typeof makeTerminal>
  connection: ReturnType<typeof makeConnection> | null
  isVisible?: boolean
  fit?: () => void
}) {
  const fitAddon = { fit: props.fit ?? vi.fn() }
  const container = makeContainer()
  return renderHook(
    (p: { connection: ReturnType<typeof makeConnection> | null; isVisible: boolean }) =>
      usePtySizeSync({
        terminal: props.terminal as unknown as SyncOptions['terminal'],
        fitAddon: fitAddon as unknown as SyncOptions['fitAddon'],
        container,
        connection: p.connection as unknown as SyncOptions['connection'],
        isVisible: p.isVisible,
      }),
    { initialProps: { connection: props.connection, isVisible: props.isVisible ?? true } },
  )
}

describe('usePtySizeSync', () => {
  it("pushes xterm's size on attach even when the fit changes nothing (pane-split remount)", () => {
    const terminal = makeTerminal(28, 200)
    const connection = makeConnection()
    render({ terminal, connection })
    expect(connection.resize).toHaveBeenCalledExactlyOnceWith(28, 200)
  })

  it("pushes when xterm's grid changes, and never for an unchanged grid", () => {
    const terminal = makeTerminal(28, 200)
    const connection = makeConnection()
    render({ terminal, connection })
    connection.resize.mockClear()

    act(() => terminal.fireResize(28, 200))
    expect(connection.resize).not.toHaveBeenCalled()

    act(() => terminal.fireResize(40, 120))
    expect(connection.resize).toHaveBeenCalledExactlyOnceWith(40, 120)
  })

  it('re-pushes to a new connection (a re-attach) even at the same size', () => {
    const terminal = makeTerminal(28, 200)
    const first = makeConnection()
    const { rerender } = render({ terminal, connection: first })
    const second = makeConnection()
    rerender({ connection: second, isVisible: true })
    expect(second.resize).toHaveBeenCalledExactlyOnceWith(28, 200)
  })

  it('a hidden view never pushes; shown, it takes the PTY size back', () => {
    const terminal = makeTerminal(28, 200)
    const connection = makeConnection()
    const { rerender } = render({ terminal, connection, isVisible: false })
    act(() => terminal.fireResize(30, 90))
    expect(connection.resize).not.toHaveBeenCalled()

    rerender({ connection, isVisible: true })
    expect(connection.resize).toHaveBeenCalledExactlyOnceWith(30, 90)
  })

  it('fits xterm to the container before reconciling', () => {
    const terminal = makeTerminal(24, 80)
    const connection = makeConnection()
    const fit = vi.fn(() => {
      terminal.rows = 50
      terminal.cols = 160
    })
    render({ terminal, connection, fit })
    expect(fit).toHaveBeenCalled()
    expect(connection.resize).toHaveBeenCalledExactlyOnceWith(50, 160)
  })
})

// The daemon's Go suite writes this fixture (TestConformance_XtermResizeFixture): per scenario, the
// keyframe before a resize, the keyframe after it, and the raw PTY output behind both.
type ResizeScenario = {
  name: string
  cols: number
  rows: number
  scrollback: number
  toCols: number
  toRows: number
  raw: string
  before: string
  after: string
  saturated: boolean
  pendingWrap: boolean
}
const resizeFixture = JSON.parse(
  readFileSync(
    join(
      dirname(fileURLToPath(import.meta.url)),
      '../../../../../../api/internal/core/terminal/internal/model/testdata/xterm-resize-reflow.json',
    ),
    'utf8',
  ),
) as { scenarios: ResizeScenario[] }

const terminals: Terminal[] = []
afterEach(() => {
  for (const terminal of terminals.splice(0)) terminal.dispose()
})

async function xtermFed(data: string, cols: number, rows: number, scrollback: number) {
  const terminal = new Terminal({ cols, rows, scrollback, allowProposedApi: true })
  terminals.push(terminal)
  await new Promise<void>((resolve) => terminal.write(data, resolve))
  return terminal
}

/** What a viewer can tell apart: every line's text, wrap bit and cell widths, and the cursor. */
function bufferOf(terminal: Terminal, exempt: { firstWrap?: boolean; cursorX?: boolean } = {}) {
  const buffer = terminal.buffer.active
  const lines = []
  for (let y = 0; y < buffer.length; y++) {
    const line = buffer.getLine(y)!
    let widths = ''
    for (let x = 0; x < terminal.cols; x++) widths += line.getCell(x)?.getWidth() ?? 1
    const wrapped = y === 0 && exempt.firstWrap ? 'exempt' : line.isWrapped
    lines.push({ text: line.translateToString(true, 0, terminal.cols), wrapped, widths })
  }
  const cursorX = exempt.cursorX ? 'exempt' : buffer.cursorX
  return { length: buffer.length, baseY: buffer.baseY, cursor: [cursorX, buffer.cursorY], lines }
}

/** Resizes a terminal the way a view does: the hook's fit, which then tells the PTY. */
function resizeThroughHook(terminal: Terminal, cols: number, rows: number) {
  const connection = makeConnection()
  renderHook(() =>
    usePtySizeSync({
      terminal,
      fitAddon: { fit: () => terminal.resize(cols, rows) } as unknown as SyncOptions['fitAddon'],
      container: makeContainer(),
      connection: connection as unknown as SyncOptions['connection'],
      isVisible: true,
    }),
  )
  expect(connection.resize).toHaveBeenCalledExactlyOnceWith(rows, cols)
}

// xterm reflows its own buffer on a resize, and the daemon reflows its model; the keyframe that
// follows must repaint exactly what the client already shows.
describe('usePtySizeSync — the keyframe after a resize matches xterm.js own reflow', () => {
  const known = 'printedSpacesKnownDivergence'
  it('the fixture covers every scenario the daemon writes', () => {
    expect(resizeFixture.scenarios.map((s) => s.name)).toContain(known)
  })

  for (const sc of resizeFixture.scenarios.filter((s) => s.name !== known)) {
    it(`${sc.name}: a client fed the keyframe, then resized, equals the next keyframe`, async () => {
      const client = await xtermFed(sc.before, sc.cols, sc.rows, sc.scrollback)
      resizeThroughHook(client, sc.toCols, sc.toRows)
      const after = await xtermFed(sc.after, sc.toCols, sc.toRows, sc.scrollback)
      // R6: no keyframe can flag its oldest line; a CUP cannot carry a pending wrap (R-6c-1).
      const exempt = { firstWrap: sc.saturated, cursorX: sc.pendingWrap }
      expect(bufferOf(client, exempt)).toEqual(bufferOf(after, exempt))
    })

    it(`${sc.name}: xterm reflowing the raw output equals the next keyframe`, async () => {
      const client = await xtermFed(sc.raw, sc.cols, sc.rows, sc.scrollback)
      resizeThroughHook(client, sc.toCols, sc.toRows)
      const after = await xtermFed(sc.after, sc.toCols, sc.toRows, sc.scrollback)
      const exempt = { firstWrap: sc.saturated }
      expect(bufferOf(client, exempt)).toEqual(bufferOf(after, exempt))
    })
  }

  // UPSTREAM.md D2: x/vt stores a printed space like an erased cell, so the daemon counts the chain
  // "x" * 20 + "   " as 20 cells; xterm counts the spaces and keeps one more row.
  it('known divergence: a chain ending in printed spaces keeps one more row on the client', async () => {
    const sc = resizeFixture.scenarios.find((s) => s.name === known)!
    const after = bufferOf(await xtermFed(sc.after, sc.toCols, sc.toRows, sc.scrollback))
    for (const data of [sc.raw, sc.before]) {
      const client = await xtermFed(data, sc.cols, sc.rows, sc.scrollback)
      resizeThroughHook(client, sc.toCols, sc.toRows)
      const got = bufferOf(client)
      expect(got.lines.slice(0, 2)).toEqual(after.lines.slice(0, 2))
      expect(got.lines[2]).toMatchObject({ text: expect.stringMatching(/^ *$/), wrapped: true })
      expect(got.lines.slice(3, 4)).toEqual(after.lines.slice(2, 3))
    }
  })
})
