import { describe, expect, it, afterEach } from 'vitest'
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

// `native/scripts/gen-extract.ts` lives outside `web/src` — it is a
// committed capture tool for the native-parity oracle, not a React source
// file — so it cannot take a `@/` alias (that only reaches `web/src`). The
// same shape already exists for `web/scripts/refresh-tree-sitter-grammars.mjs`,
// tested from `web/src/__tests__/scripts/refresh-tree-sitter-grammars.test.ts`
// via an equivalent relative import; this file follows that precedent one
// directory further up, since `native/` is a sibling of `web/` rather than
// living inside it.
import {
  buildExtractOptions,
  wrapWithPost,
  startSink,
  DEFAULT_SINK_PORT,
} from '../../../../native/scripts/gen-extract'
import { extractSnapshot, extractSnapshotSource } from '@/lib/oracle/extract'

// ─── every runtime helper `extractSnapshotSource` must inline — ORACLE_RUNTIME
// in extract.ts, copied here as a fixed list rather than imported, because the
// property under test is "gen-extract's wrapper did not drop or mangle any of
// these on the way through", which importing the same list from the same
// place the wrapper reads it from could not catch. ────────────────────────
const RUNTIME_HELPER_NAMES = [
  'oracleRound',
  'oracleNormalizeColor',
  'oracleFontWeight',
  'oracleFirstFontFamily',
  'oracleResolveLineHeight',
  'oracleMeasureNormalLineHeight',
  'oracleRelativeBounds',
  'oracleOverflowHidesText',
  'oracleIsClipped',
  'oracleContentSized',
  'oracleLineSized',
  'oracleOwnTextNodes',
  'oracleOwnText',
  'oracleTextAdvanceWidth',
  'oraclePaddingBoxRect',
  'oracleParsePx',
  'oraclePseudoBoxRect',
  'oracleGeneratesBox',
  'oracleIsVisible',
  'oracleAssertComparableOpacity',
  'oracleSurfaceScope',
  'oracleSelectDeclaredAnchors',
  'oracleDetectTheme',
  'oracleNormalizeState',
  'extractSnapshot',
]

// ── argv → ExtractOptions ───────────────────────────────────────────────────

describe('buildExtractOptions', () => {
  it('maps the simple flags onto their ExtractOptions field 1:1', () => {
    const { options, post } = buildExtractOptions([
      '--surface',
      'popover',
      '--root',
      'popover-popup',
      '--scope',
      '[data-testid=popover]',
      '--index',
      '1',
      '--post',
      'http://127.0.0.1:9999/',
    ])
    expect(options).toEqual({
      surface: 'popover',
      root: 'popover-popup',
      scope: '[data-testid=popover]',
      index: 1,
    })
    expect(post).toBe('http://127.0.0.1:9999/')
  })

  it('builds state.* from --theme/--width/--content/--flag', () => {
    const { options } = buildExtractOptions([
      '--theme',
      'dark',
      '--width',
      '600',
      '--content',
      'overflow',
      '--flag',
      'hover',
      '--flag',
      'selected',
    ])
    expect(options.state).toEqual({
      theme: 'dark',
      width: 600,
      content: 'overflow',
      flags: ['hover', 'selected'],
    })
  })

  it('builds pseudo from repeated --pseudo id=selector', () => {
    const { options } = buildExtractOptions([
      '--pseudo',
      'slider-track=::before',
      '--pseudo',
      'git-row-item=::after',
    ])
    expect(options.pseudo).toEqual({ 'slider-track': '::before', 'git-row-item': '::after' })
  })

  it('rejects an unrecognised --theme/--content before ever reaching extractSnapshotSource', () => {
    expect(() => buildExtractOptions(['--theme', 'blue'])).toThrow(/--theme/)
    expect(() => buildExtractOptions(['--content', 'huge'])).toThrow(/--content/)
  })

  it('rejects a flag missing its value, and a --pseudo without "="', () => {
    expect(() => buildExtractOptions(['--surface'])).toThrow(/--surface needs a value/)
    expect(() => buildExtractOptions(['--pseudo', 'no-equals-sign'])).toThrow(/--pseudo/)
  })

  it('rejects an unknown flag rather than silently ignoring it', () => {
    expect(() => buildExtractOptions(['--bogus', 'x'])).toThrow(/unknown argument/)
  })

  // ── --json as a base, with flags layered on top ──────────────────────────

  it('takes --json as a base and lets a later flag override one of its top-level keys', () => {
    const { options } = buildExtractOptions([
      '--json',
      '{"surface":"select","root":"select-popup"}',
      '--root',
      'select-panel',
    ])
    expect(options).toEqual({ surface: 'select', root: 'select-panel' })
  })

  it('merges state key-by-key rather than replacing the whole object', () => {
    const { options } = buildExtractOptions([
      '--json',
      '{"state":{"theme":"dark","content":"short"}}',
      '--width',
      '600',
    ])
    // theme/content came from --json and survive; width is new. If this
    // merged by wholesale object replacement instead of per-key, theme and
    // content would be gone.
    expect(options.state).toEqual({ theme: 'dark', content: 'short', width: 600 })
  })

  it('lets any --flag replace --json state.flags outright, not append to it', () => {
    const { options } = buildExtractOptions([
      '--json',
      '{"state":{"flags":["loading"]}}',
      '--flag',
      'hover',
    ])
    expect(options.state?.flags).toEqual(['hover'])
  })

  it('reads --json from a file when given @path', () => {
    const dir = mkdtempSync(join(tmpdir(), 'gen-extract-json-'))
    const file = join(dir, 'options.json')
    writeFileSync(file, JSON.stringify({ surface: 'dialog', root: 'dialog-popup' }))
    try {
      const { options } = buildExtractOptions(['--json', '@' + file])
      expect(options).toEqual({ surface: 'dialog', root: 'dialog-popup' })
    } finally {
      rmSync(dir, { recursive: true, force: true })
    }
  })

  it('reports invalid JSON by name rather than an opaque parser error', () => {
    expect(() => buildExtractOptions(['--json', '{not valid'])).toThrow(/--json is not valid JSON/)
  })
})

// ── the return-path wrapper ─────────────────────────────────────────────────

/** A minimal, capturing stand-in for the DOM's XMLHttpRequest. */
class FakeXHR {
  static instances: FakeXHR[] = []
  static respondStatus = 200
  static respondBody = ''

  method = ''
  url = ''
  async = true
  headers: Record<string, string> = {}
  body: string | undefined
  status = 0
  responseText = ''

  open(method: string, url: string, async: boolean): void {
    this.method = method
    this.url = url
    this.async = async
  }
  setRequestHeader(name: string, value: string): void {
    this.headers[name] = value
  }
  send(body?: string): void {
    this.body = body
    this.status = FakeXHR.respondStatus
    this.responseText = FakeXHR.respondBody
    FakeXHR.instances.push(this)
  }
}

describe('wrapWithPost', () => {
  afterEach(() => {
    FakeXHR.instances = []
    FakeXHR.respondStatus = 200
    FakeXHR.respondBody = ''
    Reflect.deleteProperty(globalThis, 'XMLHttpRequest')
  })

  it('embeds a syntactically valid, fully inlined runtime', () => {
    const bare = extractSnapshotSource({ surface: 'oracle-probe', root: 'probe-root' })
    const wrapped = wrapWithPost(bare, 'http://127.0.0.1:8765/')

    // A `SyntaxError` here is exactly the "ReferenceError in the page" class
    // of failure extract.ts's own module doc warns about, one step earlier:
    // this catches a mangled embed before it is ever pasted anywhere.
    expect(() => new Function(wrapped)).not.toThrow()

    for (const name of RUNTIME_HELPER_NAMES) {
      expect(wrapped).toContain(`function ${name}`)
    }
  })

  it('POSTs the exact extracted JSON synchronously and returns a small status, not the payload', () => {
    // @ts-expect-error — test double, not the real DOM constructor
    globalThis.XMLHttpRequest = FakeXHR
    const options = { surface: 'oracle-probe', root: 'probe-root' }
    document.body.innerHTML = '<div data-oracle-id="probe-root"></div>'
    const expectedJson = extractSnapshotSource(options)
    const wrapped = wrapWithPost(expectedJson, 'http://127.0.0.1:8765/mock')

    // eslint-disable-next-line no-eval
    const result = (0, eval)(wrapped) as string
    const parsed = JSON.parse(result)

    expect(parsed.ok).toBe(true)
    // The status is small and fixed-shape regardless of payload size — the
    // whole point of not returning the snapshot itself through execute_js.
    expect(Object.keys(parsed).sort()).toEqual(['bytes', 'ok'])

    expect(FakeXHR.instances).toHaveLength(1)
    const xhr = FakeXHR.instances[0]
    expect(xhr.method).toBe('POST')
    expect(xhr.url).toBe('http://127.0.0.1:8765/mock')
    expect(xhr.async).toBe(false) // synchronous — see the module's own "Why a sink"
    expect(xhr.body).toBe(JSON.stringify(extractSnapshot(options), null, 2))
    expect(parsed.bytes).toBe(xhr.body!.length)
  })

  it('reports a post failure by stage, without claiming success', () => {
    // @ts-expect-error — test double
    globalThis.XMLHttpRequest = FakeXHR
    FakeXHR.respondStatus = 500
    FakeXHR.respondBody = 'sink is down'
    document.body.innerHTML = '<div data-oracle-id="probe-root"></div>'
    const source = extractSnapshotSource({ surface: 'oracle-probe', root: 'probe-root' })
    const wrapped = wrapWithPost(source, 'http://127.0.0.1:1/')

    // eslint-disable-next-line no-eval
    const parsed = JSON.parse((0, eval)(wrapped) as string)
    expect(parsed).toEqual({ ok: false, stage: 'post', status: 500, error: 'sink is down' })
  })

  it('never attempts the POST when extraction itself throws', () => {
    // @ts-expect-error — test double
    globalThis.XMLHttpRequest = FakeXHR
    // No matching root in the document at all — extractSnapshot throws before
    // there is anything to send.
    document.body.innerHTML = ''
    const source = extractSnapshotSource({ surface: 'oracle-probe', root: 'probe-root' })
    const wrapped = wrapWithPost(source, 'http://127.0.0.1:1/')

    // eslint-disable-next-line no-eval
    const parsed = JSON.parse((0, eval)(wrapped) as string)
    expect(parsed.ok).toBe(false)
    expect(parsed.stage).toBe('extract')
    expect(FakeXHR.instances).toHaveLength(0)
  })
})

// ── the sink ─────────────────────────────────────────────────────────────────

describe('startSink', () => {
  let dir: string

  afterEach(() => {
    if (dir) rmSync(dir, { recursive: true, force: true })
  })

  it('writes the exact bytes POSTed to it, unmodified, and answers {ok,bytes}', async () => {
    dir = mkdtempSync(join(tmpdir(), 'gen-extract-sink-'))
    const outPath = join(dir, 'snapshot.json')
    // Irregular spacing and a non-ASCII character: anything that ran through
    // JSON.parse/JSON.stringify, or a text-encoding round trip, would not
    // reproduce this exactly.
    const body = '{"a":1,   "b":"日本語","c":[1,2,3]}\n'

    const handle = await startSink({ outPath, port: 0 }) // port 0: OS-assigned, collision-proof
    try {
      const res = await fetch(`http://127.0.0.1:${handle.port}/`, { method: 'POST', body })
      expect(res.status).toBe(200)
      const reply = await res.json()
      expect(reply).toEqual({ ok: true, bytes: Buffer.byteLength(body, 'utf8') })
    } finally {
      await handle.stop()
    }

    const onDisk = readFileSync(outPath)
    expect(onDisk.equals(Buffer.from(body, 'utf8'))).toBe(true)
  })

  it('stops itself after `count` POSTs, and not before', async () => {
    dir = mkdtempSync(join(tmpdir(), 'gen-extract-sink-count-'))
    const outPath = join(dir, 'snapshot.json')
    const handle = await startSink({ outPath, port: 0, count: 2 })
    // Attached before either request — the same order `sink`'s real CLI
    // uses (it registers this the moment `startSink` resolves, long before
    // any POST arrives). Attaching it after the fetches below would race:
    // the server can close while still inside the second request's own
    // handler, and a 'close' listener added afterwards would miss an event
    // that already fired.
    const closed = new Promise<void>((resolve) => handle.server.on('close', resolve))

    await fetch(`http://127.0.0.1:${handle.port}/`, { method: 'POST', body: 'one' })
    // Still up for the second.
    const second = await fetch(`http://127.0.0.1:${handle.port}/`, { method: 'POST', body: 'two' })
    expect(second.status).toBe(200)

    expect(readFileSync(outPath, 'utf8')).toBe('two')

    await closed
    expect(handle.server.listening).toBe(false)
  })

  it('rejects a non-POST with 405 rather than silently accepting it', async () => {
    dir = mkdtempSync(join(tmpdir(), 'gen-extract-sink-method-'))
    const outPath = join(dir, 'snapshot.json')
    const handle = await startSink({ outPath, port: 0, count: 1 })
    try {
      const res = await fetch(`http://127.0.0.1:${handle.port}/`, { method: 'GET' })
      expect(res.status).toBe(405)
    } finally {
      await handle.stop()
    }
  })

  it('answers CORS preflight so a cross-origin page can reach it', async () => {
    dir = mkdtempSync(join(tmpdir(), 'gen-extract-sink-cors-'))
    const outPath = join(dir, 'snapshot.json')
    const handle = await startSink({ outPath, port: 0, count: 1 })
    try {
      const res = await fetch(`http://127.0.0.1:${handle.port}/`, { method: 'OPTIONS' })
      expect(res.headers.get('access-control-allow-origin')).toBe('*')
    } finally {
      await handle.stop()
    }
  })

  it('picks a documented default port well clear of 5173/5180/9223/9224', () => {
    expect(DEFAULT_SINK_PORT).toBe(8765)
    expect([5173, 5180, 9223, 9224]).not.toContain(DEFAULT_SINK_PORT)
  })
})

// ── the round trip that matters: CLI args → injected source → snapshot ─────
//
// Everything above tests gen-extract's own logic in isolation. This is the
// one that proves the whole pipe: build ExtractOptions the way the CLI would
// from real argv, generate the injectable the same way `emit` does, run it
// against a DOM under jsdom, and check it against `extractSnapshot` called
// directly on the identical DOM. A drift anywhere in `buildExtractOptions` —
// including one that silently captures the wrong element — shows up here.

describe('the CLI round trip: argv -> injected source -> snapshot', () => {
  afterEach(() => {
    document.body.innerHTML = ''
    document.documentElement.className = ''
  })

  function mountTwoScopes() {
    document.documentElement.className = 'light'
    // Two candidate roots for the same id, one inside each scope, sized
    // differently — so a scope bug (picking the wrong element, or the whole
    // document) is caught by geometry, not just by "did it throw".
    document.body.innerHTML = `
      <div id="scope-a"><div id="a" data-oracle-id="probe-root" style="width:10px"></div></div>
      <div id="scope-b"><div id="b" data-oracle-id="probe-root" style="width:20px"></div></div>`
    const a = document.getElementById('a') as HTMLElement
    const b = document.getElementById('b') as HTMLElement
    for (const [el, w] of [
      [a, 10],
      [b, 20],
    ] as const) {
      Object.defineProperty(el, 'getBoundingClientRect', {
        configurable: true,
        value: () => ({
          x: 0,
          y: 0,
          left: 0,
          top: 0,
          right: w,
          bottom: 5,
          width: w,
          height: 5,
          toJSON: () => ({}),
        }),
      })
    }
  }

  it('produces exactly what extractSnapshot produces from the same DOM, argv scope and all', () => {
    mountTwoScopes()
    const argv = ['--surface', 'oracle-probe', '--root', 'probe-root', '--scope', '#scope-b']
    const { options } = buildExtractOptions(argv)

    const source = extractSnapshotSource(options)
    // eslint-disable-next-line no-eval
    const emitted = JSON.parse((0, eval)(source) as string)
    const direct = JSON.parse(JSON.stringify(extractSnapshot(options)))

    expect(emitted).toEqual(direct)
    // And it is specifically the scoped element (width 20, #scope-b), not
    // the other candidate or the whole document — the property `--scope`
    // exists to guarantee.
    expect(emitted.anchors[0].bounds.w).toBe(20)
  })

  it('is sensitive to which scope was named — the control for the test above', () => {
    mountTwoScopes()
    const { options: optsA } = buildExtractOptions([
      '--surface',
      'oracle-probe',
      '--root',
      'probe-root',
      '--scope',
      '#scope-a',
    ])
    const { options: optsB } = buildExtractOptions([
      '--surface',
      'oracle-probe',
      '--root',
      'probe-root',
      '--scope',
      '#scope-b',
    ])

    const snapA = JSON.parse((0, eval)(extractSnapshotSource(optsA)) as string)
    const snapB = JSON.parse((0, eval)(extractSnapshotSource(optsB)) as string)

    expect(snapA.anchors[0].bounds.w).toBe(10)
    expect(snapB.anchors[0].bounds.w).toBe(20)
  })
})
