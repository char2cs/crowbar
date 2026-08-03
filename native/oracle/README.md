# `native/oracle/` — the parity differ, and how its reference half gets captured

This crate (`Cargo.toml` above) is the differ: two JSON snapshots in, ranked
deltas out. `ANCHORS.md` is the contract both snapshots satisfy, and
`corpus/`/`runs/` are its append-only fixture and archive directories — none
of the three are what this file is about.

This file is about the **other** half: how the *reference* snapshot — the
React side's answer, captured live from the running Tauri app's webview — gets
from `web/src/lib/oracle/extract.ts` onto disk in the first place. That tool
is `native/scripts/gen-extract.ts`, and it has existed twice before as an
uncommitted scratchpad pair (a `gen-extract.ts` plus a `Bun.serve` sink) that
evaporated both times a session ended. This file exists so the recipe survives
a third time — read `gen-extract.ts`'s own header comment for the full option
reference; what follows is the same loop, shorter.

## The loop

Nothing here drives the app for you. A live Tauri instance and its MCP bridge
have to already be running, and their real bridge port has to be known —
`native/QUEUE.md` records that the default (9223) and the actually-used port
(often 9224) disagree, so check the running instance's own log rather than
assume.

1. **Start the sink**, in its own terminal, before anything else:

   ```sh
   bun native/scripts/gen-extract.ts sink --out /tmp/p3-ref-<surface>.json --port 8765
   ```

   It prints the port and output path, then blocks until it has received
   `--count` POSTs (default 1) or is stopped with Ctrl-C.

2. **Generate the injectable**, in a second terminal:

   ```sh
   bun native/scripts/gen-extract.ts emit \
     --surface <surface> --root <root-id> --scope '<css selector>' \
     --post http://127.0.0.1:8765/ > /tmp/inject.js
   ```

   `--scope` narrows the walk to one element — `extract.ts`'s own doc comment
   explains why it has to be a plain CSS selector string rather than an
   `Element`. For a surface small enough that the bridge's own return value is
   reliable (a handful of anchors — `number-input.md` §"Capture evidence"
   measured this working directly), drop `--post` and redirect the bare
   output instead; see `gen-extract.ts`'s "Why a sink" section for exactly
   where that stops being safe.

3. **Paste `/tmp/inject.js`'s contents into the running app** through the
   execute_js bridge (e.g. `mcp__tauri__webview_execute_js`, at the real
   bridge port from step 0). With `--post` set, the call returns almost
   immediately with something like `{"ok":true,"bytes":4213}` — confirmation
   the sink has already written the file — or `{"ok":false,"stage":…}` naming
   which half failed.

4. **Read back the file** the sink wrote — `jq . /tmp/p3-ref-<surface>.json`
   or similar — before trusting it. At the default `--count 1` the sink has
   already exited by this point.

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

## What this replaces

Every `native/mapping/*.md` account of a live capture through this path —
`dialog.md` §6, `sidebar.md` §7, `input.md` §10 — describes the same shape by
hand: "written to disk byte-exact through a local HTTP sink (`Bun.serve` on an
ephemeral port, body straight to `writeFileSync`), so nothing round-tripped
through the bridge's JSON." That was true every time and reproducible none of
them, because the tool itself was never committed. It is now.
