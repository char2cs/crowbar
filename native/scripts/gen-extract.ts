#!/usr/bin/env bun
/**
 * gen-extract.ts — turns `web/src/lib/oracle/extract.ts`'s
 * `extractSnapshotSource(options)` into something a human can actually run
 * against the live app, and gets its answer back onto disk.
 *
 * This has existed twice before as an ad-hoc scratchpad pair
 * (`gen-extract.ts` + a `Bun.serve` sink) and evaporated both times because
 * neither was committed. Every `native/mapping/*.md` §6/§7/§10 capture
 * account depends on the workflow below having existed; this file — and
 * `native/oracle/README.md`, which restates the same loop — is where it now
 * lives instead of a chat transcript.
 *
 * `extractSnapshotSource` itself is NOT reimplemented or modified here. This
 * script only (1) turns CLI arguments into the `ExtractOptions` object it
 * takes, (2) optionally wraps its output so the payload travels over HTTP
 * instead of through the bridge's return channel, and (3) runs a small local
 * HTTP sink that writes what it receives to disk, byte for byte.
 *
 * ── P3.46 correction: this is the FALLBACK path, not the default one ───────
 *
 * Driving the live Tauri app found that `--post` (and so `sink`) cannot be
 * reached at all from inside this app's dev webview: `fetch`/`XMLHttpRequest`
 * from `http://localhost:5173` to any other port fails with `Load failed`,
 * for both GET and POST — the webview's own cross-origin scoping, not a CSP
 * policy this project authors (the document carries no CSP `<meta>` tag) or
 * anything a response header could fix. A sink on any other port can never
 * receive a POST from this page.
 *
 * The primary capture path for this app, in dev, is a direct
 * `import('/src/lib/oracle/extract.ts')` from inside the page — Vite serves
 * the real module at the app's own origin, so there is no cross-origin
 * request and no stringified copy to keep in sync. See
 * `native/oracle/README.md` for that recipe. Everything below — `emit`,
 * `sink`, `--post` — remains correct as a mechanism and is still what you
 * want for a context where the module genuinely cannot be imported: a
 * packaged build with no dev server, or a capture driven from a different
 * host entirely.
 *
 * ── Usage ──────────────────────────────────────────────────────────────────
 *
 *   bun native/scripts/gen-extract.ts emit [options]
 *   bun native/scripts/gen-extract.ts sink --out <path> [--port <n>] [--count <n>]
 *   bun native/scripts/gen-extract.ts --help
 *
 * `emit` is the default: `bun native/scripts/gen-extract.ts --surface foo …`
 * works without the word `emit` in front.
 *
 * emit options — each maps onto one `ExtractOptions` field (see
 * `web/src/lib/oracle/extract.ts`'s own doc comment for what each means):
 *
 *   --surface <name>                    options.surface
 *   --root <id>                         options.root
 *   --scope <selector>                  options.scope — a CSS selector string.
 *                                        `extractSnapshotSource` refuses anything
 *                                        else (an Element can't survive being
 *                                        serialised into injectable source), and
 *                                        this tool does not soften that: every
 *                                        value handed to `--scope` is already a
 *                                        string by construction, and a `--json`
 *                                        base that puts something else there is
 *                                        passed straight through to that refusal.
 *   --index <n>                         options.index
 *   --theme <light|dark>                options.state.theme
 *   --width <n>                         options.state.width
 *   --content <short|normal|overflow>   options.state.content
 *   --flag <name>                       options.state.flags — repeatable
 *   --pseudo <id>=<selector>            options.pseudo — repeatable
 *   --json <json | @path>               a base ExtractOptions object; every
 *                                        flag above overrides the matching key
 *                                        (state and pseudo merge key-by-key
 *                                        rather than replacing the object,
 *                                        except --flag, which replaces the
 *                                        whole flags array once any --flag is
 *                                        given). @path reads the JSON from a
 *                                        file instead of the argument itself.
 *   --post <url>                        wrap the emitted script so it POSTs the
 *                                        JSON to <url> — normally a `sink`
 *                                        started in another terminal — and
 *                                        returns a small `{ok,bytes}` status
 *                                        instead of the payload. See "Why a
 *                                        sink" below — and the P3.46 note at
 *                                        the top of this file: this cannot
 *                                        reach a sink from inside this app's
 *                                        own dev webview at all.
 *
 * sink options:
 *
 *   --out <path>     required. Where the received body is written, verbatim.
 *   --port <n>       default 8765. Clear of 5173 (the main Vite dev server),
 *                    5180 (`bun run dev:mock`, used for some captures —
 *                    slider.md), and 9223/9224 (the Tauri MCP bridge —
 *                    QUEUE.md's own port-collision trap). Override if 8765 is
 *                    ever taken too.
 *   --count <n>      default 1. The sink stops itself after this many POSTs.
 *                    Ctrl-C stops it sooner, at any time — "startable and
 *                    stoppable on demand" is exactly this: start it, it runs
 *                    until it has what it came for or you tell it to quit.
 *
 * ── The sink loop, end to end (fallback — see the P3.46 note above) ───────
 *
 * For this app, in dev, don't start here — use `native/oracle/README.md`'s
 * direct-`import()` loop instead. This loop is for a context where that
 * doesn't apply (a packaged build, or a different host).
 *
 * Assume nothing has been done yet — a live Tauri app is already running (do
 * not start or restart it) and its MCP bridge port is known (QUEUE.md: it
 * defaults to 9223 in tooling but the app itself often logs 9224 — read the
 * real port out of the running instance's log before trusting either).
 *
 *   1. Start the sink, in its own terminal, before doing anything else:
 *
 *        bun native/scripts/gen-extract.ts sink --out /tmp/p3-ref-foo.json --port 8765
 *
 *      It prints a line confirming the port and the output path, then blocks.
 *
 *   2. Generate the injectable, in a second terminal:
 *
 *        bun native/scripts/gen-extract.ts emit \
 *          --surface foo --root foo-root --scope '[data-testid=foo]' \
 *          --post http://127.0.0.1:8765/ > /tmp/inject.js
 *
 *      (Drop `--post …` — and just redirect the bare `emit` output — for a
 *      small surface where the return value alone is reliable; see "Why a
 *      sink" for when that stops being true.)
 *
 *   3. Paste the contents of `/tmp/inject.js` into the running app's webview
 *      through the execute_js bridge (e.g. `mcp__tauri__webview_execute_js`,
 *      pointed at the real bridge port from step 0). With `--post` set, the
 *      call returns almost immediately with something like
 *      `{"ok":true,"bytes":4213}` — that is confirmation the sink has already
 *      written the file, not a promise that it will. `{"ok":false,…}` means
 *      it did not, and says at which stage (`extract` or `post`) and why.
 *
 *   4. Back in the first terminal, the sink has written `/tmp/p3-ref-foo.json`
 *      and (at the default `--count 1`) already exited. Read it back —
 *      `jq . /tmp/p3-ref-foo.json` or similar — before trusting it.
 *
 * ── Why a sink, and why a synchronous XHR inside the page ────────────────
 *
 * `execute_js` has a documented failure mode for exactly this payload:
 * QUEUE.md records `webview_execute_js` timing out at 7s when asked to
 * return a large `JSON.stringify` of a whole snapshot — the injected code
 * runs fine, the *return* is what does not survive. Trimming the snapshot to
 * dodge that would throw away the thing being captured; instead, the return
 * value is made small on purpose. The injected script does its own write —
 * an HTTP POST to a sink this tool starts locally — and hands `execute_js`
 * back only a status object, which is small regardless of how large the
 * surface is.
 *
 * The POST is a **synchronous** `XMLHttpRequest`, not `fetch`. That is not
 * incidental: this project's own notes record that the execute_js bridge
 * does not await a returned promise, so an in-flight `fetch` racing the
 * bridge's own return has no way to prove — from outside the page — whether
 * it ever completed. A synchronous XHR blocks the injected script (and so
 * the execute_js call) until the POST has actually succeeded or failed, so
 * the `{"ok":true,…}` this tool hands back is a fact about a file that
 * already exists, not a hope about one that might. `Content-Type:
 * text/plain` is deliberate too: it keeps the POST inside the CORS-safelisted
 * request shape, so no preflight `OPTIONS` round trip has to complete first —
 * the sink still answers `OPTIONS` correctly, in case a future browser or a
 * future header disagrees.
 *
 * None of the above helps from inside this app's own dev webview (P3.46):
 * the request never gets far enough for the `Content-Type: text/plain` /
 * no-preflight care above to matter — it fails with `Load failed` before any
 * CORS negotiation happens at all, which is the webview's own cross-origin
 * scoping rather than anything on the HTTP layer. This section documents a
 * mechanism that is still correct for a host where the request can be made
 * in the first place; it's not the reason a capture from this app succeeds
 * or fails today.
 *
 * This mirrors what the prior, uncommitted `gen-extract.ts` + `Bun.serve`
 * pair already did, per `native/mapping/dialog.md` §6 and the same note in
 * `sidebar.md` §7 and `input.md` §10: "written to disk byte-exact through a
 * local HTTP sink … so nothing round-tripped through the bridge's JSON." The
 * sink here is `node:http` rather than literally `Bun.serve` — chosen so the
 * exact same code path is exercisable from `vitest` (which does not
 * guarantee it is running under the Bun engine) as well as from `bun
 * native/scripts/gen-extract.ts sink` — but the shape and the guarantee are
 * the same: the bytes it writes are the bytes it received, via
 * `fs.writeFileSync`, with nothing in between that could reformat them.
 */

import { createServer, type IncomingMessage, type Server, type ServerResponse } from 'node:http'
import { readFileSync, writeFileSync } from 'node:fs'
import {
  extractSnapshotSource,
  type ExtractOptions,
  type OracleState,
} from '../../web/src/lib/oracle/extract'

// ─── emit: CLI arguments → ExtractOptions ─────────────────────────────────

export interface EmitArgs {
  options: ExtractOptions
  /** `--post <url>`, if given. */
  post?: string
}

/**
 * Turns `emit`'s argv into the `ExtractOptions` `extractSnapshotSource` takes,
 * plus the `--post` URL if one was given. Pure and exported so it is testable
 * without spawning a process — see `gen-extract.test.ts`'s CLI-parsing suite.
 *
 * `--json` supplies a base object; every other flag overrides the matching
 * key on top of it. `state` and `pseudo` are merged key-by-key rather than
 * replaced wholesale, so `--json '{"state":{"theme":"dark"}}' --width 600`
 * keeps the declared theme and adds the width, rather than one silently
 * discarding the other. `--flag` is the one exception: any `--flag` given
 * replaces `--json`'s `state.flags` outright rather than appending to it,
 * because "the set this capture declares" should not silently accumulate
 * flags from an unrelated base file a caller happened to reuse.
 */
export function buildExtractOptions(argv: string[]): EmitArgs {
  // `Partial`, not `ExtractOptions`: the type requires `surface`, but
  // `extractSnapshot`/`extractSnapshotSource` do not — a missing one is
  // treated as `'unknown'` (see extract.ts's own `opts.surface || 'unknown'`)
  // — and this CLI does not force `--surface` any harder than the module it
  // wraps does. The cast at the bottom of this function is that same
  // leniency, made explicit in one place instead of implicitly everywhere.
  let jsonBase: Partial<ExtractOptions> = {}
  let post: string | undefined
  const overrides: Partial<ExtractOptions> = {}
  const stateOverrides: Partial<OracleState> = {}
  const flags: string[] = []
  let sawFlags = false
  const pseudoOverrides: Record<string, string> = {}
  let sawPseudo = false

  for (let i = 0; i < argv.length; i++) {
    const arg = argv[i]
    const need = (): string => {
      const v = argv[++i]
      if (v === undefined) throw new Error(`gen-extract: ${arg} needs a value`)
      return v
    }

    switch (arg) {
      case '--json': {
        const raw = need()
        const text = raw.startsWith('@') ? readFileSync(raw.slice(1), 'utf8') : raw
        let parsed: unknown
        try {
          parsed = JSON.parse(text)
        } catch (e) {
          throw new Error(`gen-extract: --json is not valid JSON: ${String((e as Error).message)}`)
        }
        jsonBase = parsed as Partial<ExtractOptions>
        break
      }
      case '--surface':
        overrides.surface = need()
        break
      case '--root':
        overrides.root = need()
        break
      case '--scope':
        // Always a plain CLI string, which is exactly the one shape
        // `extractSnapshotSource` accepts for `scope` — see its own doc
        // comment for why an Element cannot be. Nothing here needs to
        // re-enforce that; there is no code path by which this flag could
        // produce anything but a string.
        overrides.scope = need()
        break
      case '--index': {
        const v = need()
        const n = Number(v)
        if (!Number.isFinite(n)) {
          throw new Error(`gen-extract: --index must be a number, got ${JSON.stringify(v)}`)
        }
        overrides.index = n
        break
      }
      case '--theme': {
        const v = need()
        if (v !== 'light' && v !== 'dark') {
          throw new Error(`gen-extract: --theme must be "light" or "dark", got ${JSON.stringify(v)}`)
        }
        stateOverrides.theme = v
        break
      }
      case '--width': {
        const v = need()
        const n = Number(v)
        if (!Number.isFinite(n)) {
          throw new Error(`gen-extract: --width must be a number, got ${JSON.stringify(v)}`)
        }
        stateOverrides.width = n
        break
      }
      case '--content': {
        const v = need()
        if (v !== 'short' && v !== 'normal' && v !== 'overflow') {
          throw new Error(
            `gen-extract: --content must be "short", "normal" or "overflow", got ${JSON.stringify(v)}`,
          )
        }
        stateOverrides.content = v
        break
      }
      case '--flag':
        flags.push(need())
        sawFlags = true
        break
      case '--pseudo': {
        const kv = need()
        const eq = kv.indexOf('=')
        if (eq < 0) {
          throw new Error(`gen-extract: --pseudo needs "id=selector", got ${JSON.stringify(kv)}`)
        }
        pseudoOverrides[kv.slice(0, eq)] = kv.slice(eq + 1)
        sawPseudo = true
        break
      }
      case '--post':
        post = need()
        break
      default:
        throw new Error(`gen-extract: unknown argument ${JSON.stringify(arg)} (see --help)`)
    }
  }

  const mergedState: Partial<OracleState> | undefined =
    jsonBase.state || Object.keys(stateOverrides).length > 0 || sawFlags
      ? {
          ...jsonBase.state,
          ...stateOverrides,
          // `OracleFlag` is a fixed, closed vocabulary; a `--flag` value is
          // arbitrary CLI text and the type checker cannot narrow it any
          // further than `string`. Left unchecked here on purpose, the same
          // way extract.ts's own test suite exercises caller-supplied state
          // ("a value arriving from an injected script, where there is no
          // type checker at all"): `oracleNormalizeState`, reached through
          // `extractSnapshotSource` below, is the single place the allowed
          // vocabulary is declared, and it throws by name on anything else.
          // Duplicating that list here would be a second copy to keep in
          // sync rather than a second check worth having.
          ...(sawFlags ? { flags: flags as OracleState['flags'] } : {}),
        }
      : undefined

  const mergedPseudo: Record<string, string> | undefined =
    jsonBase.pseudo || sawPseudo ? { ...jsonBase.pseudo, ...pseudoOverrides } : undefined

  const options = {
    ...jsonBase,
    ...overrides,
    ...(mergedState ? { state: mergedState } : {}),
    ...(mergedPseudo ? { pseudo: mergedPseudo } : {}),
  } as ExtractOptions

  return { options, post }
}

// ─── the return-path wrapper ────────────────────────────────────────────────

/**
 * Wraps a bare `extractSnapshotSource(...)` string so the injected script
 * POSTs its own JSON to `url` via a synchronous `XMLHttpRequest` and hands
 * `execute_js` back a small `{ok, bytes}` (or `{ok:false, stage, error}`)
 * status instead of the payload. See the module doc comment ("Why a sink…")
 * for the reasoning.
 *
 * `source` is embedded as a sub-expression, not substituted into a template
 * string: `extractSnapshotSource`'s own output is already a complete,
 * self-evaluating `(function () { … })()` expression, so no escaping is
 * needed or attempted. That also means a bug that truncates or otherwise
 * mangles `source` on the way in fails as a `SyntaxError` the moment the
 * result is parsed — loud, not a silently wrong runtime value.
 */
export function wrapWithPost(source: string, url: string): string {
  return (
    '(function () {\n' +
    '  var __json;\n' +
    '  try {\n' +
    '    __json = ' +
    source +
    '\n' +
    '  } catch (e) {\n' +
    '    return JSON.stringify({ ok: false, stage: "extract", error: String((e && e.message) || e) });\n' +
    '  }\n' +
    '  try {\n' +
    '    var __xhr = new XMLHttpRequest();\n' +
    '    __xhr.open("POST", ' +
    JSON.stringify(url) +
    ', false);\n' + // false: synchronous — see "Why a sink" above
    '    __xhr.setRequestHeader("Content-Type", "text/plain;charset=UTF-8");\n' +
    '    __xhr.send(__json);\n' +
    '    if (__xhr.status < 200 || __xhr.status >= 300) {\n' +
    '      return JSON.stringify({ ok: false, stage: "post", status: __xhr.status, error: __xhr.responseText });\n' +
    '    }\n' +
    '  } catch (e) {\n' +
    '    return JSON.stringify({ ok: false, stage: "post", error: String((e && e.message) || e) });\n' +
    '  }\n' +
    '  return JSON.stringify({ ok: true, bytes: __json.length });\n' +
    '})()'
  )
}

// ─── sink ───────────────────────────────────────────────────────────────────

/** Well clear of 5173 (vite), 5180 (`dev:mock`), and 9223/9224 (MCP bridge). */
export const DEFAULT_SINK_PORT = 8765

export interface SinkOptions {
  /** Where the received body is written, byte for byte. */
  outPath: string
  port?: number
  hostname?: string
  /** How many POSTs to accept before the server closes itself. Default 1. */
  count?: number
  onWrite?: (info: { bytes: number; path: string }) => void
}

export interface SinkHandle {
  server: Server
  /** The actual bound port — meaningful when `port: 0` asked for an OS-assigned one. */
  port: number
  stop: () => Promise<void>
}

const CORS_HEADERS: Record<string, string> = {
  'access-control-allow-origin': '*',
  'access-control-allow-methods': 'POST, OPTIONS',
  'access-control-allow-headers': 'Content-Type',
}

/**
 * A local HTTP sink: POST a body, it lands at `outPath` unmodified via
 * `fs.writeFileSync`, and the server tells the caller `{ok:true,bytes}`.
 * Stops itself once `count` POSTs have landed (default 1) — "startable and
 * stoppable on demand" is this, plus `handle.stop()` / Ctrl-C at any time.
 *
 * `node:http` rather than `Bun.serve`, deliberately: this function is
 * exercised directly from `vitest`, which is not guaranteed to be running
 * under the Bun engine even when launched via `bun run test`, and
 * `Bun.serve` does not exist outside it. `node:http` is implemented by both
 * Bun and Node, so the exact code path this test suite exercises is the one
 * `bun native/scripts/gen-extract.ts sink` runs for real.
 */
export function startSink(opts: SinkOptions): Promise<SinkHandle> {
  const wanted = opts.count ?? 1
  let received = 0

  const server = createServer((req: IncomingMessage, res: ServerResponse) => {
    if (req.method === 'OPTIONS') {
      res.writeHead(204, CORS_HEADERS)
      res.end()
      return
    }
    if (req.method !== 'POST') {
      res.writeHead(405, { ...CORS_HEADERS, 'content-type': 'text/plain' })
      res.end('gen-extract sink: expected POST')
      return
    }

    const chunks: Buffer[] = []
    req.on('data', (chunk: Buffer) => chunks.push(chunk))
    req.on('error', (err: Error) => {
      res.writeHead(400, { ...CORS_HEADERS, 'content-type': 'text/plain' })
      res.end(String(err))
    })
    req.on('end', () => {
      const body = Buffer.concat(chunks)
      // Byte for byte: no JSON.parse/stringify round trip, no toString()
      // through a text encoding, no trailing anything — what arrived is
      // exactly what gets written.
      writeFileSync(opts.outPath, body)
      received++
      opts.onWrite?.({ bytes: body.length, path: opts.outPath })

      const lastOne = received >= wanted
      res.writeHead(200, {
        ...CORS_HEADERS,
        'content-type': 'text/plain',
        // On the last expected POST, tell the client not to keep this socket
        // open for reuse — otherwise a keep-alive client can hold the
        // connection past `res.end()`, and `server.close()` below waits for
        // every open connection to end before its callback fires, which
        // would make "stops itself after `count` POSTs" hang rather than
        // stop. `closeAllConnections()` right after is the same guarantee
        // for a client that ignores the header anyway.
        ...(lastOne ? { connection: 'close' } : {}),
      })
      res.end(JSON.stringify({ ok: true, bytes: body.length }))

      if (lastOne) {
        // Let this response flush before the socket goes away.
        setImmediate(() => {
          server.close()
          server.closeAllConnections()
        })
      }
    })
  })

  return new Promise((resolve, reject) => {
    server.once('error', reject)
    server.listen(opts.port ?? DEFAULT_SINK_PORT, opts.hostname ?? '127.0.0.1', () => {
      const address = server.address()
      const port =
        address && typeof address === 'object' ? address.port : (opts.port ?? DEFAULT_SINK_PORT)
      resolve({
        server,
        port,
        stop: () =>
          new Promise<void>((res) => {
            server.close(() => res())
            server.closeAllConnections()
          }),
      })
    })
  })
}

// ─── CLI plumbing ───────────────────────────────────────────────────────────

const HELP = `gen-extract — emit the oracle's injectable extractor, and receive its answer.

  bun native/scripts/gen-extract.ts emit [options]
  bun native/scripts/gen-extract.ts sink --out <path> [--port <n>] [--count <n>]
  bun native/scripts/gen-extract.ts --help

This is the fallback capture path (a packaged build, or a host other than
this app's own dev webview — see this file's header comment, "P3.46
correction"). For this app in dev, native/oracle/README.md's direct
import()-from-the-page loop is what actually works; this file's header
comment still has the full option list and the sink-based recipe for when
that loop doesn't apply.`

function runEmitCli(argv: string[]): void {
  const { options, post } = buildExtractOptions(argv)
  const bare = extractSnapshotSource(options)
  process.stdout.write((post ? wrapWithPost(bare, post) : bare) + '\n')
}

async function runSinkCli(argv: string[]): Promise<void> {
  let outPath: string | undefined
  let port = DEFAULT_SINK_PORT
  let count = 1

  for (let i = 0; i < argv.length; i++) {
    const arg = argv[i]
    const need = (): string => {
      const v = argv[++i]
      if (v === undefined) throw new Error(`gen-extract sink: ${arg} needs a value`)
      return v
    }
    switch (arg) {
      case '--out':
        outPath = need()
        break
      case '--port':
        port = Number(need())
        break
      case '--count':
        count = Number(need())
        break
      default:
        throw new Error(`gen-extract sink: unknown argument ${JSON.stringify(arg)} (see --help)`)
    }
  }
  if (!outPath) throw new Error('gen-extract sink: --out <path> is required')

  const handle = await startSink({
    outPath,
    port,
    count,
    onWrite: ({ bytes, path }) => {
      process.stderr.write(`gen-extract sink: wrote ${bytes} bytes to ${path}\n`)
    },
  })
  process.stderr.write(
    `gen-extract sink: listening on http://127.0.0.1:${handle.port}/ ` +
      `→ ${outPath} (stops after ${count} POST${count === 1 ? '' : 's'}; Ctrl-C to stop sooner)\n`,
  )

  const shutdown = (): void => {
    handle.stop().then(() => process.exit(0))
  }
  process.on('SIGINT', shutdown)
  process.on('SIGTERM', shutdown)

  await new Promise<void>((resolve) => handle.server.on('close', resolve))
  process.stderr.write('gen-extract sink: stopped\n')
}

async function main(): Promise<void> {
  const argv = process.argv.slice(2)
  const [first, ...rest] = argv

  if (first === undefined || first === '--help' || first === '-h') {
    if (first === undefined) {
      // No arguments at all is almost certainly a caller who meant --help,
      // not a caller who meant "emit with every ExtractOptions default" —
      // the latter is reachable with `emit` explicitly.
      process.stderr.write(HELP + '\n')
      process.exitCode = 1
      return
    }
    process.stdout.write(HELP + '\n')
    return
  }
  if (first === 'sink') {
    await runSinkCli(rest)
    return
  }
  if (first === 'emit') {
    runEmitCli(rest)
    return
  }
  // No subcommand named — treat the whole argv as `emit`'s, so
  // `--surface foo --root foo-root` works without typing `emit` first.
  runEmitCli(argv)
}

if (import.meta.main) {
  main().catch((err: unknown) => {
    process.stderr.write(String((err as Error)?.message ?? err) + '\n')
    process.exit(1)
  })
}
