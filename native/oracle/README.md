# `native/oracle/` — the parity differ, and how its reference half gets captured

This crate (`Cargo.toml` above) is the differ: two JSON snapshots in, ranked
deltas out. `ANCHORS.md` is the contract both snapshots satisfy, and
`corpus/`/`runs/` are its append-only fixture and archive directories — none
of the three are what this file is about.

This file is about the **other** half: how the *reference* snapshot — the
React side's answer, captured live from the running Tauri app's webview — gets
from `web/src/lib/oracle/extract.ts` onto disk in the first place.

**P3.46 correction:** the loop below was rewritten after driving the live app
against it for the first time. Two of its assumptions didn't survive contact:
the `--post` sink (previously documented as the default return path) cannot be
reached from this app's webview at all, and a snapshot depends on app state
the capture options say nothing about. Both are covered below, in "Why not the
sink" and "The trap: app state isn't in `state`".

## The native half needs **two** things, and they fail identically

Before any of the reference-capture loop below matters, the *native* snapshot
has to exist. It takes two independent switches, and **missing either one
produces the exact same symptom**: `crowbar-app` starts, prints its banner and
its loaded fonts to stderr, opens an ordinary window, and never exits. No
error, no JSON, no clue which switch is missing.

```sh
CROWBAR_ROW_SNAPSHOT=- \
  cargo run -p crowbar-app --bin crowbar-app --features driver -- --surface <name> ...
```

1. **`--features driver`.** Without it `main.rs`'s `#[cfg(not(feature =
   "driver"))] fn open` is what runs, and that function's whole job is to open
   the gate surface for a human to look at.
2. **`CROWBAR_ROW_SNAPSHOT`.** With the feature but no destination,
   `row_snapshot::Destination::from_env()` returns `None`, no registry is
   installed, and the driver build falls through to the same window. `-` means
   stdout; anything else is a file path.

This has now cost two separate sessions. The first blamed a nonexistent
capture regression on a build missing switch 1; this one had switch 1 right
and switch 2 missing, and re-diagnosed the identical hang from scratch through
four rebuilds. **Check the switches before the code**, and if you build a
control to compare against, build it the same way — a control that differs in
either switch tells you nothing.

`cargo` also reports `Finished` for an already-fresh artifact, so a green build
log is not evidence the binary on disk has the feature. `touch`ing the source
does not reliably invalidate the fingerprint either. The only trustworthy probe
is behavioural: run it and see whether JSON comes out.

## The loop

Nothing here drives the app for you. A live Tauri instance and its MCP bridge
have to already be running, and their real bridge port has to be known —
`native/QUEUE.md` records that the default (9223) and the actually-used port
(often 9224) disagree, so check the running instance's own log rather than
assume.

The primary path is a **direct `import()` of the real module**, not the
injected, stringified copy `native/scripts/gen-extract.ts` builds. In dev,
Vite serves `web/src/lib/oracle/extract.ts` at the app's own origin
(`http://localhost:5173/src/lib/oracle/extract.ts`), so a script running in the
page can just import it. That is the whole reason this is better than
injection: there is exactly one copy of the extractor, so it cannot drift from
what the page is actually running, and none of `extractSnapshotSource`'s
`ORACLE_RUNTIME` bookkeeping (the fixed list it has to keep in sync by hand) is
relevant — the import gets every helper for free, from the same file, every
time.

The one wrinkle: the `execute_js` bridge does **not** await a returned
promise, and `import()` is always async. So this is two calls, not one.

1. **Arm it** — paste this through the execute_js bridge (e.g.
   `mcp__tauri__webview_execute_js`, at the real bridge port). It starts the
   import and, once it resolves, runs the capture and stashes the result on
   `window`:

   ```js
   window.__cap = undefined
   import('/src/lib/oracle/extract.ts').then((m) => {
     try {
       window.__cap = m.extractSnapshot({
         surface: 'sidebar-header',
         root: 'sidebar-header',
         state: { theme: 'light' },
       })
     } catch (e) {
       window.__cap = { __error: String((e && e.message) || e) }
     }
   })
   ```

   This call returns almost immediately — before the import or the capture
   have necessarily finished. That is expected; it only confirms the script
   started, not that `window.__cap` is populated yet.

2. **Read it back** — a second execute_js call, a beat later:

   ```js
   JSON.stringify(window.__cap)
   ```

   If this comes back `undefined`, the promise from step 1 hadn't settled
   yet; retry the same read. A `{"__error": "..."}` shape means
   `extractSnapshot` threw — the message says why (unknown surface, root not
   found, a theme mismatch against the document, etc. — see `extract.ts`'s own
   throws). Otherwise this is the snapshot, ready to write to
   `/tmp/p3-ref-<surface>.json` byte for byte.

Because `extractSnapshot` (unlike the injected `extractSnapshotSource` path)
takes a real `Element` for `scope` as well as a selector string, step 1 can
also `document.querySelector(...)` something itself and hand the element
straight to `extractSnapshot` — the CSS-selector-only restriction on
`gen-extract.ts`'s `--scope` exists specifically because *that* path has to
survive serialisation into source text; the direct-import path never
serialises anything, so it doesn't need the restriction.

`native/scripts/gen-extract.ts`'s CLI (`buildExtractOptions`, `--surface`,
`--root`, `--state`, etc.) is still the right reference for what each
`ExtractOptions` field means, and its `emit`/`sink`/`--post` machinery still
works as a tool in its own right — see "Why not the sink" below for where it
still applies. What it is **not**, any more, is the default way to get the
answer back out of *this* app's page; that's the two calls above.

## Why not the sink

`gen-extract.ts`'s `emit`/`sink`/`--post` machinery — POST the snapshot to a
local HTTP server, so a large payload doesn't have to survive the
`execute_js` return channel — was the original design, and it still exists.
**Driving the live app found it does not work from inside this app's dev
webview, at all:**

The app's origin is `http://localhost:5173`. From the page, `fetch(...)` (and
XHR) to `http://127.0.0.1:<sink-port>` fails with `Load failed` — for both GET
and POST. There is no CSP `<meta>` tag in the document, and the sink itself
already answers CORS correctly (`access-control-allow-origin: *`, proved by
its own test in `gen-extract.test.ts`) — so this isn't a policy we author or a
header the sink could add to fix it. It's the webview's own cross-origin
scoping, underneath CORS. **A sink on any other port can never receive a POST
(or a GET) from this page, no matter what it answers with.**

So `--post` is not the reliable route here, and should not be reached for
first. It remains useful for a context where the direct-import path above
doesn't apply — a packaged build with no dev server serving `extract.ts` at
the app's origin, or a capture driven from some other host entirely. For
*this* app, in dev, use the two-call loop above.

If that context does apply, the shape is still what it always was:

1. **Start the sink**, in its own terminal, before anything else:

   ```sh
   bun native/scripts/gen-extract.ts sink --out /tmp/p3-ref-<surface>.json --port 8765
   ```

2. **Generate the injectable**, in a second terminal:

   ```sh
   bun native/scripts/gen-extract.ts emit \
     --surface <surface> --root <root-id> --scope '<css selector>' \
     --post http://127.0.0.1:8765/ > /tmp/inject.js
   ```

3. **Paste `/tmp/inject.js`'s contents into the running app** through the
   execute_js bridge. With `--post` set, the call returns almost immediately
   with something like `{"ok":true,"bytes":4213}` or `{"ok":false,"stage":…}`.

4. **Read back the file** the sink wrote — `jq . /tmp/p3-ref-<surface>.json`
   or similar — before trusting it.

### And `width` defaults to the root, not the viewport

Omit `width` from `ExtractOptions.state` and the extractor fills it from the
**root anchor's own width**. On `project-home-row` that produced `width: 332`
— the surface — against a real viewport of `855`. It is the same
`--width`/`--viewport-width` confusion this port keeps re-learning on the
native side, arriving from the React side for the first time, and it is worse
here because nothing looks wrong: the field is populated, plausible, and
matches the surface's own geometry.

Two ways it hurts. The differ **refuses** when the two `state` blocks
disagree, which is the good case — loud and immediate. The bad case is
driving the native side to *match* the fabricated number, at which point both
snapshots agree on a cell that describes neither app. Always pass
`width: window.innerWidth` explicitly.

## The trap: app state isn't in `state`

`ExtractOptions.state` (width, theme, content, flags) records the §8.3 matrix
cell a capture is supposed to represent. It does **not** record anything about
*which* app state the live document happens to be in when the capture runs —
which sidebar carousel page is scrolled into view, which dialog is open, which
tab is selected. A capture taken in the wrong one of those can differ from the
correct reference by a single field and look, in every other respect,
identical — which is exactly the shape of a false port defect.

**Worked example.** A `sidebar-header` capture matched the stored reference in
every field but one:

```
sidebar-header.visible:  ref=true   mine=false      ← everything else byte-equal
```

Cause: there is exactly one `sidebar-header` in the DOM, and it lives inside
`carousel-panel-files` — one of the sidebar carousel's panels
(`native/MAPPING.md`'s table of `carousel-panel-workspaces` / `-chats` /
`-files` / `-git`, rooted at `carousel-scrollport`). That panel was two pages
off-screen: `x: 2058` in a 1714px viewport. `oracleIsVisible` was correctly
reporting `false` for an element that was, in fact, not visible — the
extractor wasn't wrong, the *app state at capture time* didn't match the app
state the reference was captured in.

Setting `[data-oracle-id="carousel-scrollport"]`'s `scrollLeft` to `688`
brought the panel to `x: 1370` (inside the viewport), and the re-capture then
matched the stored reference **byte-for-byte**.

**The fix has a fixed shape:** before capturing, drive whatever app state the
surface depends on — scroll the right container into view, open the right
dialog, select the right tab — the same way any other live interaction would,
and record *what you did* next to the surface, not inside the JSON (the
schema is closed; ANCHORS.md v1.1 makes an unknown field a hard failure, so
this can't be smuggled into the snapshot itself). A `native/mapping/*.md`
account of a capture is the right place for that sentence.

## Why a sink exists at all

`webview_execute_js` has a documented failure mode for exactly this payload:
`native/QUEUE.md` records it timing out at 7s when the return value is a large
`JSON.stringify` of a whole snapshot — the injected code runs fine, only the
*return* does not survive. Trimming the snapshot to dodge that would throw
away the thing being captured, so instead the return value is made small on
purpose: the injected script does its own write, over an HTTP POST to a sink
this tool starts locally, and hands the bridge back only a status object.
`gen-extract.ts`'s header comment has the fuller account, including why the
POST is a **synchronous** `XMLHttpRequest` rather than `fetch`.

That reasoning is still sound *as a mechanism* — it just can't reach this
app's webview (see "Why not the sink" above). Every reference captured so far
has stayed small enough (under ~2.6 KB) for the direct-import loop's step 2 to
return whole; nothing here has yet needed a payload too large for that read.
If one turns up, note that the sink can't help with it from inside this app
either — that's an open question, not a solved one.

## What this replaces

Every `native/mapping/*.md` account of a live capture through this path —
`dialog.md` §6, `sidebar.md` §7, `input.md` §10 — describes the same shape by
hand: "written to disk byte-exact through a local HTTP sink (`Bun.serve` on an
ephemeral port, body straight to `writeFileSync`), so nothing round-tripped
through the bridge's JSON." That was true every time and reproducible none of
them, because the tool itself was never committed. It is now — as a fallback;
the loop at the top of this file is what a fresh capture should actually run.
