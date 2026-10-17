# S0 Ratchet Gates Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Three dependency-free Node gates (file-size cap, pattern counters, per-module LOC budgets) whose baselines can only shrink, wired into `make pr-checks` and CI, so every later refactor stage ratchets instead of drifting.

**Architecture:** One shared `lib.mjs` (file loading, line counting, baseline diff, check/update driver) and three small scripts that each produce a flat `Record<string, number>` and hand it to the driver. The driver compares it to a committed flat JSON baseline: any growth fails, any shrink fails until written back with `--update`, and `--update` itself refuses to raise a number. Same shape as the existing `deadcode-check.sh` and `check-test-names.sh` gates, in Node so it runs identically in the Go job, the web job and a laptop without `bun`.

**Tech Stack:** Node >= 20 built-ins only (`node:test`, `node:fs`, `node:child_process`), `git ls-files`, GNU make, GitHub Actions.

**Spec:** `docs/plans/2026-10-16-crowbar-architecture.md` §11 row S0 ("Per-module LOC budgets, timer/`_ =`/empty-catch counters, 500-line cap for new files"; gate "Baselines may only shrink; budgets fail CI on growth"), §1 (the measured symptoms), and the "What done looks like" table at the end of §11 for the numbers the budgets ratchet toward.

## Global Constraints

- Work from the worktree root (`/Users/char2cs/.crowbar/projects/cb81ea03-54b7-4093-8bdd-fdd6b183e91d/github.com/char2cs/crowbar/fix/refactor-chats/worktree`, branch `fix/refactor-chats`). Do not push, open PRs, or use bare `git stash`.
- Disk is nearly full: no builds, no `go test`, no vitest, no full suites. Everything here is `node --test scripts/ratchet/*.test.mjs` and the three gate scripts, which allocate nothing. Stop and report if a task seems to need a build.
- Spec numbers are targets, never gate values: Go source 127k -> <= 60k, web source 106k -> <= 55k, files > 500 lines 65 -> 0, discarded Go errors 155 -> 0, empty TS catches -> 0, client timers 87 -> <= 15.
- "Baselines only shrink" (CLAUDE.md): no task may add or raise a baseline entry by hand. The one allowed manual edit is re-keying (below).
- Shipped as one release: no flags, no compat shims, no parallel old/new paths. These gates replace nothing, so nothing is deleted.
- No new dependency. Scripts use no semicolons, single quotes, 2-space indent (matches `web/scripts/*.mjs`). Comments explain why in <= 3 lines.
- Tests live beside the script as `scripts/ratchet/<unit>.test.mjs` and run with `node --test` (`web/src/__tests__/` is the vitest tree and does not apply to root scripts). Tests use in-memory fixtures and temp dirs, never the live repo contents, no sleeps.
- Commit messages end with the session's `Co-Authored-By` trailer. If the dispatcher commits on the worker's behalf, use the message given in the task.

## Measured today (HEAD `9641256b5`, clean tree)

Source set = tracked `api/**/*.go` minus `*_test.go` (includes `api/tests/kit`, mocks) and tracked `web/src/**/*.{ts,tsx}` minus `web/src/__tests__/`. This reproduces the architecture doc's 127k / 106k exactly. Physical lines, `wc -l` semantics (count of `\n`).

| Measure | Today | Doc says | Note |
|---|---:|---:|---|
| Go source lines | 127,962 | 127k | 858 files |
| Web source lines | 106,009 | 106k | |
| Files > 500 lines, source | 64 (Go 33 + web 31) | 65 | within one of the doc; test files add 129 more (Go 86, web 43) |
| Go timers (`time.After/NewTimer/NewTicker/AfterFunc/Tick`) | 29 | n/a | |
| Go discarded errors (`_ =`, `_, _ =`) | 183 | 155 | doc excludes `api/tests/kit` and the `_, _ =` form; both discard errors, gate counts both |
| Web timers (`setTimeout`/`setInterval`) | 87 | 87 | |
| Web empty `catch` blocks (body empty or comments only) | 34 | n/a | |
| Web swallowed promises (`.catch(() => {})`, `=> undefined/null/void 0`) | 32 | n/a | |

Ruling on "empty catch": the doc's 111 "bare TS catches" is the count of binding-less `catch {` of any body, most of which handle the error. The gate counts only handlers that do nothing (34 + 32 = 66), because that is what §6 forbids and what S3 drives to 0.

Where each key is heading (informational, not encoded anywhere): `go.total` -> 60k (S7), `web.total` -> 55k (S9), `go.wiring` 4,403 -> 400 (S7), `go.mocks` 2,658 -> 0 and `go.dto` 2,165 -> 0 (S7), `web.chat-surface` 21,130 -> 12k (S2), `web.sidebar` 18,562 -> 5k (S4), `web.extensions` 609 -> 0 (S9), `go.discarded-errors`, `web.empty-catch`, `web.swallowed-promise` -> 0 (S3), `web.timers` -> 15, oversize files -> 0.

## Review Focus

Failure modes the spec implies but a happy-path build would not exercise; each has a test in the owning task.

1. A new directory or file under `api/` or `web/src/` that no module claims must fail loudly, not drop out of the budget silently (Task 3 `measureLoc` unassigned test).
2. CRLF files and files without a trailing newline count the same lines as `wc -l` (Task 1 `countLines` test).
3. An oversize file that is renamed or moved shows as both `gone` and `new`; the failure text must tell the author how to re-key it instead of looking like a bug (Task 1 `runGate` test asserts the hint).
4. A regex gate that over-counts is worse than none: gofmt-aligned `_        = T{}` blocks and `catch` blocks with a body must not count; comment-only catch bodies must (Task 2 tests).
5. A local `make ratchet` before `git add` must see new untracked files, and a tracked file deleted from disk must not crash the run (Task 1 `loadCode` test in a temp git repo).

Known residual, accepted: nothing mechanical stops someone editing a baseline upward to match grown code (develop will not carry these files until this branch merges, so a base-ref comparison would have nothing to compare to). `--update` refuses to raise, and every stage's review diffs `scripts/ratchet/*-baseline.json` and requires every changed number to go down.

## File Structure

| File | Responsibility |
|---|---|
| `scripts/ratchet/lib.mjs` | Load gated files, count lines, diff measured vs baseline, check/update driver |
| `scripts/ratchet/file-size.mjs` | Gate 1: files over 500 lines (per-file baseline, new ones fail outright) |
| `scripts/ratchet/counters.mjs` | Gate 2: timers, discarded Go errors, empty/swallowed catches (totals) |
| `scripts/ratchet/loc-budgets.mjs` | Gate 3: source lines per module and per language total |
| `scripts/ratchet/modules.json` | Path prefix -> module map for gate 3 (config, edited when code moves) |
| `scripts/ratchet/{file-size,counters,loc-budgets}-baseline.json` | Committed baselines, flat sorted `{ "key": number }` |
| `scripts/ratchet/*.test.mjs` | One `node:test` file per script plus `lib.test.mjs` |
| `Makefile`, `.github/workflows/ci.yml`, `CLAUDE.md` | Wiring (Task 4) |

Dispatch order: Task 1, then Tasks 2 and 3 (independent, disjoint files), then Task 4.

---

### Task 1: Shared driver and the 500-line gate

**Files:**
- Create: `scripts/ratchet/lib.mjs`, `scripts/ratchet/lib.test.mjs`
- Create: `scripts/ratchet/file-size.mjs`, `scripts/ratchet/file-size.test.mjs`
- Create (generated): `scripts/ratchet/file-size-baseline.json`

**Interfaces:**
- Produces, from `lib.mjs`:
  - `REPO_ROOT: string` (absolute path of the repo root, derived from `import.meta.url`).
  - `countLines(text: string): number` (number of `\n` characters).
  - `classify(path: string): { lang: 'go' | 'web'; test: boolean } | null`. `go` = under `api/`, ends `.go`, `test` = ends `_test.go`. `web` = under `web/src/`, ends `.ts` or `.tsx`, `test` = under `web/src/__tests__/`. Anything else is `null`.
  - `type CodeFile = { path: string; lang: 'go' | 'web'; test: boolean; text: string }`.
  - `loadCode(root?: string): CodeFile[]`. Runs `git ls-files -z --cached --others --exclude-standard -- api web/src` with `cwd: root` (default `REPO_ROOT`), dedupes, keeps paths where `classify` is non-null, reads each file as utf8, skips (does not throw on) paths missing from disk.
  - `type Change = { key: string; was?: number; now?: number }`.
  - `diffBaseline(baseline: Record<string, number>, measured: Record<string, number>): { grew: Change[]; added: Change[]; shrunk: Change[]; gone: Change[] }`. Key in both with `now > was` -> `grew`, `now < was` -> `shrunk`; only in `measured` -> `added`; only in `baseline` -> `gone`. Each list sorted by key.
  - `runGate(opts: { name: string; baselinePath: string; measured: Record<string, number>; update: boolean; allowAdded: boolean; summary: string; hint?: string; log?: (line: string) => void }): number` returning the process exit code (0/1). `log` defaults to `console.log`.
- Produces, from `file-size.mjs`: `MAX_LINES = 500`, `measureOversize(files: CodeFile[], max?: number): Record<string, number>` (path -> lines for every file in `files`, source and test alike, with lines `> max`).

**`runGate` behaviour (the contract the tests pin):**
- Check mode (`update: false`):
  - baseline file missing: log `<name>: no baseline at <baselinePath>; run: make ratchet ARGS=--update`, return 1.
  - empty diff: log `<name>: ok (<summary>)`, return 0.
  - otherwise log one line per change (`  grew   <key>: <was> -> <now>`, `  new    <key>: <now>`, `  shrank <key>: <was> -> <now>`, `  gone   <key>: was <was>`) and return 1. `grew`, and `added` when `allowAdded` is false, are the *growth* group; `shrunk`, `gone`, and `added` when `allowAdded` is true are the *write-back* group. After the lines, print the growth trailer `Baselines never go up: delete or split code.` (plus `hint` if given) when the growth group is non-empty, and the write-back trailer `Write the shrink back: make ratchet ARGS=--update, then commit the baseline.` when the write-back group is non-empty (both, if both).
- Update mode (`update: true`):
  - baseline file missing: write `measured`, log `<name>: baseline created (<n> keys)`, return 0.
  - `grew` non-empty, or `added` non-empty with `allowAdded` false: write nothing, log the offending lines then `<name>: refusing to raise a baseline`, return 1.
  - empty diff: log `<name>: already current`, return 0, file untouched.
  - otherwise replace the file with `measured` and log `<name>: baseline updated (<n> keys)`, return 0.
- File format: `JSON.stringify(sortedByKey, null, 2) + '\n'`, keys sorted by code unit.

- [ ] **Step 1: Write the failing tests**

`scripts/ratchet/lib.test.mjs` (imports `node:test`, `node:assert/strict`, `node:fs`, `node:os`, `node:path`, `node:child_process`):

```js
test('countLines counts newline characters like wc -l', () => {
  assert.equal(countLines(''), 0)
  assert.equal(countLines('a\nb\n'), 2)
  assert.equal(countLines('a\nb'), 1)
  assert.equal(countLines('a\r\nb\r\n'), 2)
})

test('classify admits only Go and web/src TypeScript and marks tests', () => {
  assert.deepEqual(classify('api/internal/x.go'), { lang: 'go', test: false })
  assert.deepEqual(classify('api/internal/x_test.go'), { lang: 'go', test: true })
  assert.deepEqual(classify('web/src/a/b.tsx'), { lang: 'web', test: false })
  assert.deepEqual(classify('web/src/__tests__/a/b.test.ts'), { lang: 'web', test: true })
  for (const p of ['web/src/index.css', 'web/vite.config.ts', 'api/go.mod', 'desktop/src-tauri/src/lib.rs', 'scripts/ratchet/lib.mjs'])
    assert.equal(classify(p), null)
})

test('diffBaseline buckets every kind of change and is empty when equal', () => {
  assert.deepEqual(diffBaseline({ a: 1 }, { a: 1 }), { grew: [], added: [], shrunk: [], gone: [] })
  const d = diffBaseline({ up: 1, down: 5, old: 3 }, { up: 2, down: 4, fresh: 9 })
  assert.deepEqual(d.grew, [{ key: 'up', was: 1, now: 2 }])
  assert.deepEqual(d.shrunk, [{ key: 'down', was: 5, now: 4 }])
  assert.deepEqual(d.added, [{ key: 'fresh', now: 9 }])
  assert.deepEqual(d.gone, [{ key: 'old', was: 3 }])
})
```

`runGate` tests use `mkdtempSync(join(tmpdir(), 'ratchet-'))` for the baseline path and a `log` that pushes to an array. Cases, each its own `test(...)`:
- check with no baseline file returns 1 and the log mentions `--update`.
- check with equal baseline returns 0 and logs a line starting `<name>: ok`.
- check with growth returns 1, logs `grew`, and does not mention writing back.
- check with a shrink returns 1 and the log contains `make ratchet ARGS=--update`.
- check with `added` and `allowAdded: false` returns 1 and the log contains the passed `hint`; with `allowAdded: true` it returns 1 and the log contains `make ratchet ARGS=--update`.
- an oversize rename (baseline `{ 'old.go': 600 }`, measured `{ 'new.go': 600 }`, `hint: 'Renamed an oversize file? Change its key in the baseline, value unchanged.'`) logs both `gone   old.go` and `new    new.go` and the word `Renamed`.
- update with no baseline writes the file (sorted keys, trailing newline) and returns 0.
- update refuses growth: returns 1, file bytes unchanged, log contains `refusing to raise`.
- update refuses `added` when `allowAdded` is false, accepts it when true.
- update with a shrink and a gone key rewrites the file to exactly `measured` and returns 0; a second update logs `already current`.

`loadCode` test (Review Focus 5): in a temp dir run `git init -q`, create `api/a.go` and `api/b_test.go`, `git add api/a.go`, delete `api/a.go` from disk, create untracked `api/c.go` and untracked ignored `api/ignored.go` (with a `.gitignore` listing it) and `web/src/d.ts`; assert `loadCode(tmp).map(f => f.path).sort()` equals `['api/b_test.go', 'api/c.go', 'web/src/d.ts']` and that `b_test.go` has `test: true`. (`b_test.go` is untracked too, so it is picked up by `--others`.)

`scripts/ratchet/file-size.test.mjs`:

```js
const f = (path, n, test = false) => ({ path, lang: path.startsWith('api/') ? 'go' : 'web', test, text: 'x\n'.repeat(n) })
test('MAX_LINES is 500', () => assert.equal(MAX_LINES, 500))
test('measureOversize keeps files strictly over the cap, tests included', () => {
  const out = measureOversize([f('api/a.go', 500), f('api/b.go', 501), f('web/src/__tests__/c.test.ts', 900, true), f('web/src/d.ts', 12)])
  assert.deepEqual(out, { 'api/b.go': 501, 'web/src/__tests__/c.test.ts': 900 })
})
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `node --test scripts/ratchet/*.test.mjs`
Expected: FAIL, `Cannot find module` for `./lib.mjs` / `./file-size.mjs`.

- [ ] **Step 3: Implement `scripts/ratchet/lib.mjs`** with the exports and the `runGate` contract above. `countLines` must not split the string (files are large): walk with `indexOf('\n')`.

- [ ] **Step 4: Implement `scripts/ratchet/file-size.mjs`.** Exports `MAX_LINES` and `measureOversize`; a `main(args)` that calls `loadCode()`, `measureOversize`, then `runGate({ name: 'file-size', baselinePath: <dir>/file-size-baseline.json, measured, update: args.includes('--update'), allowAdded: false, summary: '<n> oversize files baselined: go src a, go tests b, web src c, web tests d' (buckets from `classify` on each key's path), hint: 'Split the file below 500 lines. Renamed an oversize file? Change its key in the baseline, value unchanged.' })`. Run `main` and `process.exit` on its return only when the module is the entry point (`import.meta.url === pathToFileURL(process.argv[1]).href`), so tests can import it. Header comment, <= 3 lines: why per-file (a count lets one file grow while another shrinks) and that new files over the cap fail because they are never in the baseline.

- [ ] **Step 5: Run the tests to verify they pass**

Run: `node --test scripts/ratchet/*.test.mjs`
Expected: all pass, 0 fail.

- [ ] **Step 6: Create the baseline and check it**

Run: `node scripts/ratchet/file-size.mjs --update`
Expected: `file-size: baseline created (193 keys)`.

Run: `node scripts/ratchet/file-size.mjs`
Expected: `file-size: ok (193 oversize files baselined: go src 33, go tests 86, web src 31, web tests 43)`. If the counts differ, the tree changed since `9641256b5`; confirm with `git log 9641256b5..HEAD --stat -- api web/src` that the difference is real before accepting.

- [ ] **Step 7: Prove it fails on a new oversize file**

Run: `node -e "require('fs').writeFileSync('web/src/zz-probe.ts', 'x\n'.repeat(501))" && node scripts/ratchet/file-size.mjs; echo "exit=$?"; rm web/src/zz-probe.ts`
Expected: output contains `new    web/src/zz-probe.ts: 501` and the split hint, `exit=1`.

Run: `git status --short`
Expected: only `scripts/ratchet/` files listed (the probe is gone).

- [ ] **Step 8: Commit**

```bash
git add scripts/ratchet/lib.mjs scripts/ratchet/lib.test.mjs scripts/ratchet/file-size.mjs scripts/ratchet/file-size.test.mjs scripts/ratchet/file-size-baseline.json
git commit -m "ci(ratchet): fail new files over 500 lines, baseline the existing 193"
```

---

### Task 2: Pattern counters

**Files:**
- Create: `scripts/ratchet/counters.mjs`, `scripts/ratchet/counters.test.mjs`
- Create (generated): `scripts/ratchet/counters-baseline.json`

**Interfaces:**
- Consumes (Task 1): `loadCode`, `runGate`, `CodeFile`.
- Produces: `COUNTERS: { id: string; lang: 'go' | 'web'; re: RegExp }[]` and `measureCounters(files: CodeFile[]): Record<string, number>`. The result always contains every counter id (value 0 when absent). Only files with `test === false` and `lang === counter.lang` are scanned; the count is the number of regex matches summed over files.

**Counters (exact; ids are the baseline keys):**

| id | lang | regex |
|---|---|---|
| `go.timers` | go | `/\btime\.(?:After\|NewTimer\|NewTicker\|AfterFunc\|Tick)\(/g` |
| `go.discarded-errors` | go | `/^[ \t]*_(?:, _)* = /gm` |
| `web.timers` | web | `/\b(?:setTimeout\|setInterval)\b/g` |
| `web.empty-catch` | web | `/\bcatch\b\s*(?:\([^)]*\))?\s*\{(?:\s\|\/\/[^\n]*\|\/\*[\s\S]*?\*\/)*\}/g` |
| `web.swallowed-promise` | web | `/\.catch\(\s*(?:\([^)]*\)\|[\w$]+)\s*=>\s*(?:\{(?:\s\|\/\/[^\n]*\|\/\*[\s\S]*?\*\/)*\}\|undefined\|null\|void 0)\s*\)/g` |

(`\|` above is table escaping; write plain `|` in code.) `go.discarded-errors` is deliberately single-space: it skips gofmt-aligned blocks such as `_        = models.Telemetry{}` (a compile-time assertion, not a discard). `web.empty-catch` treats a body of only whitespace and comments as empty, so a `// ignore` does not hide a swallow.

- [ ] **Step 1: Write the failing tests** in `counters.test.mjs`. Helper `const file = (lang, text, test = false) => ({ path: 'x', lang, test, text })`. One `test(...)` per counter, each asserting `measureCounters([...])[id]`:
  - `go.timers`: `time.After(d)`, `time.NewTimer(d)`, `time.NewTicker(d)`, `time.AfterFunc(d, f)` each count (4); `time.Now()`, `context.WithTimeout(` and `clock.After(` do not.
  - `go.discarded-errors`: `\t_ = f.Close()` and `\t_, _ = fmt.Fprintf(w, "x")` count (2); `var _ Iface = (*T)(nil)`, `x, _ := m[k]`, `_        = models.Telemetry{}` and `// _ = f.Close()` do not.
  - `web.timers`: `setTimeout(fn, 1)`, `window.setTimeout(fn, 1)`, `setInterval(fn, 1)` count (3); `clearTimeout(id)` does not.
  - `web.empty-catch`: `try { a() } catch {}`, `catch (e) {}`, multi-line `catch {\n  // ignore\n}` and `catch { /* nope */ }` count (4); `catch (e) { log(e) }` and `.catch(handle)` do not.
  - `web.swallowed-promise`: `p.catch(() => {})`, `p.catch((e) => {})`, `p.catch(() => undefined)`, `p.catch(() => null)`, `p.catch(() => { /* ignore */ })` count (5); `p.catch((e) => report(e))` and `p.catch(() => { retry() })` do not.
  - scoping: Go text containing `setTimeout(` adds nothing to `web.timers`; a `test: true` file is ignored; every id is present with 0 for an empty input.

- [ ] **Step 2: Run to verify failure**

Run: `node --test scripts/ratchet/counters.test.mjs`
Expected: FAIL, `Cannot find module './counters.mjs'`.

- [ ] **Step 3: Implement `scripts/ratchet/counters.mjs`**: `COUNTERS`, `measureCounters` (use `text.match(re)?.length ?? 0`; `String.prototype.match` with a `g` regex resets `lastIndex`, so the shared regex objects are safe), and an entry-point `main` calling `runGate({ name: 'counters', baselinePath: <dir>/counters-baseline.json, measured, update, allowAdded: true, summary: '<id> <n>, ...' })`. `allowAdded: true` lets a later stage introduce a counter with its own baseline in one commit. Header comment <= 3 lines: totals not per-file so file moves do not churn the baseline; regex counts are approximate by design (a `catch` inside a string would count).

- [ ] **Step 4: Run to verify pass**

Run: `node --test scripts/ratchet/counters.test.mjs`
Expected: all pass.

- [ ] **Step 5: Create the baseline and check it**

Run: `node scripts/ratchet/counters.mjs --update && node scripts/ratchet/counters.mjs`
Expected baseline `scripts/ratchet/counters-baseline.json` equal to (diff any difference against `git log 9641256b5..HEAD`):

```json
{
  "go.discarded-errors": 183,
  "go.timers": 29,
  "web.empty-catch": 34,
  "web.swallowed-promise": 32,
  "web.timers": 87
}
```

and the second command prints `counters: ok (...)`.

- [ ] **Step 6: Prove growth fails and a shrink must be written back**

Run: `printf 'export const t = setTimeout(() => {}, 1)\n' > web/src/zz-probe.ts && node scripts/ratchet/counters.mjs; echo "exit=$?"; rm web/src/zz-probe.ts`
Expected: `grew   web.timers: 87 -> 88` and `exit=1`.

Run: `node -e "const f='scripts/ratchet/counters-baseline.json',j=JSON.parse(require('fs').readFileSync(f));j['web.timers']=90;require('fs').writeFileSync(f,JSON.stringify(j,null,2)+'\n')" && node scripts/ratchet/counters.mjs; echo "exit=$?"`
Expected: `shrank web.timers: 90 -> 87`, the text `make ratchet ARGS=--update`, `exit=1`.

Run: `node scripts/ratchet/counters.mjs --update && node scripts/ratchet/counters.mjs`
Expected: `counters: baseline updated (5 keys)` then `counters: ok (...)`, and the file again shows `"web.timers": 87`.

- [ ] **Step 7: Commit**

```bash
git add scripts/ratchet/counters.mjs scripts/ratchet/counters.test.mjs scripts/ratchet/counters-baseline.json
git commit -m "ci(ratchet): count timers, discarded errors and swallowed catches; baselines only shrink"
```

---

### Task 3: Per-module LOC budgets

**Files:**
- Create: `scripts/ratchet/loc-budgets.mjs`, `scripts/ratchet/loc-budgets.test.mjs`
- Create: `scripts/ratchet/modules.json`
- Create (generated): `scripts/ratchet/loc-budgets-baseline.json`

**Interfaces:**
- Consumes (Task 1): `loadCode`, `countLines`, `runGate`, `CodeFile`.
- Produces: `type Modules = { go: Record<string, string[]>; web: Record<string, string[]> }` and `measureLoc(files: CodeFile[], modules: Modules): Record<string, number>`.
  - Only files with `test === false` count. A file belongs to the module owning its longest matching prefix; prefix `p` matches path `f` when `f === p || f.startsWith(p + '/')` (so a prefix can be a directory or one exact file, and `api/internal/app` does not match `api/internal/application/x.go`).
  - Keys: `<lang>.<module>` per module (0 when empty) and `<lang>.total` (sum of the language's files).
  - A source file of a language that matches no prefix throws `Error('unassigned source files (add them to scripts/ratchet/modules.json):\n  <path>...')` listing every such path.
- `main` reads `modules.json` beside the script, calls `runGate({ name: 'loc-budgets', baselinePath: <dir>/loc-budgets-baseline.json, measured, update, allowAdded: true, summary: 'go <total>, web <total>' })`, and prints an unassigned error then exits 1. `allowAdded: true` is what lets a new module appear (its number is its measured size, and `<lang>.total` must still not rise because `--update` refuses growth).

**Re-keying rule (put verbatim in the script's header comment, <= 3 lines):** modules map prefixes to budgets; to move code between modules edit both budgets by hand in the same commit and never raise `<lang>.total`; `--update` only lowers.

`modules.json` (the module boundaries; names follow the §2 layer map, Go prefixes are today's directories, longest prefix wins so the `wiring` files carve out of their packages):

```json
{
  "go": {
    "wiring": [
      "api/internal/adapter/container.go",
      "api/internal/api/container.go",
      "api/internal/api/v0/container.go",
      "api/internal/app/container.go",
      "api/internal/app/repositories/container.go",
      "api/internal/app/usecases/container.go",
      "api/internal/engine/container.go"
    ],
    "domain": ["api/internal/domain"],
    "chat": [
      "api/internal/app/usecases/chat",
      "api/internal/app/repositories/chat",
      "api/internal/api/v0/endpoints/chat",
      "api/internal/api/v0/endpoints/threads",
      "api/internal/adapter/store/agentjournal"
    ],
    "engine-provider": ["api/internal/engine/agents", "api/internal/engine/provider", "api/internal/engine/mcp"],
    "engine-other": ["api/internal/engine"],
    "service": ["api/internal/app/usecases"],
    "mocks": ["api/internal/app/usecases/mocks"],
    "store": ["api/internal/adapter", "api/internal/app/repositories", "api/internal/app/tree"],
    "stream": [
      "api/internal/app/hub",
      "api/internal/app/realtime",
      "api/internal/api/v0/ws",
      "api/internal/api/v0/snapshots.go"
    ],
    "dto": ["api/internal/api/v0/dto"],
    "http": ["api/internal/api"],
    "core": ["api/internal/core"],
    "app-misc": ["api/internal/app", "api/internal/internal.go", "api/internal/perf"],
    "cmd": ["api/cmd"],
    "test-support": ["api/tests", "api/internal/testutil"]
  },
  "web": {
    "chat-surface": ["web/src/features/agent"],
    "sidebar": [
      "web/src/components/sidebar",
      "web/src/components/layout",
      "web/src/components/tree-dnd",
      "web/src/lib/store"
    ],
    "editor": ["web/src/features/editor", "web/src/components/editor"],
    "kit-ui": ["web/src/components/ui"],
    "workspace-panes": [
      "web/src/features/panes",
      "web/src/features/tabs",
      "web/src/features/workspace",
      "web/src/features/window",
      "web/src/features/layout"
    ],
    "git-review": ["web/src/features/git"],
    "settings": ["web/src/features/settings"],
    "files": ["web/src/features/file-explorer", "web/src/features/file-system", "web/src/features/files"],
    "terminal": ["web/src/features/terminal", "web/src/features/console"],
    "core": [
      "web/src/lib",
      "web/src/utils",
      "web/src/hooks",
      "web/src/types",
      "web/src/components/app-sync-engine.ts",
      "web/src/components/app-sync-provider.tsx"
    ],
    "app-shell": [
      "web/src/main.tsx",
      "web/src/routeTree.gen.ts",
      "web/src/vite-env.d.ts",
      "web/src/routes",
      "web/src/components/oobe",
      "web/src/components/projects",
      "web/src/components/error-boundary.tsx",
      "web/src/features/keymaps"
    ],
    "extensions": ["web/src/extensions"]
  }
}
```

- [ ] **Step 1: Write the failing tests** in `loc-budgets.test.mjs`. Helper `f(path, lines, test = false)` building a `CodeFile` of `lines` newlines with `lang` from the path prefix.
  - `longest prefix wins`: modules `{ go: { svc: ['api/internal/app'], mocks: ['api/internal/app/mocks'], wiring: ['api/internal/app/container.go'] }, web: {} }`; files `api/internal/app/a.go` (10), `api/internal/app/mocks/m.go` (20), `api/internal/app/container.go` (30) give `go.svc` 10, `go.mocks` 20, `go.wiring` 30, `go.total` 60.
  - `prefix respects directory boundaries`: `api/internal/application/x.go` is not matched by prefix `api/internal/app` (it is reported unassigned, see below).
  - `test files do not count`: an `api/x_test.go` and a `web/src/__tests__/y.test.ts` add nothing to any key.
  - `totals are per language and modules with no files are 0`: web file counts only under `web.*`; a declared empty module yields its key with 0.
  - `unassigned source file throws and names every offender` (Review Focus 1): `assert.throws(() => measureLoc([f('api/new/x.go', 5), f('web/src/new/y.ts', 5)], modules), /unassigned source files[\s\S]*api\/new\/x\.go[\s\S]*web\/src\/new\/y\.ts/)`.

- [ ] **Step 2: Run to verify failure**

Run: `node --test scripts/ratchet/loc-budgets.test.mjs`
Expected: FAIL, `Cannot find module './loc-budgets.mjs'`.

- [ ] **Step 3: Implement `scripts/ratchet/loc-budgets.mjs`** (`measureLoc`, entry-point `main`) and create `scripts/ratchet/modules.json` exactly as above.

- [ ] **Step 4: Run to verify pass**

Run: `node --test scripts/ratchet/loc-budgets.test.mjs`
Expected: all pass.

- [ ] **Step 5: Create the baseline and check it**

Run: `node scripts/ratchet/loc-budgets.mjs --update && node scripts/ratchet/loc-budgets.mjs`
Expected `scripts/ratchet/loc-budgets-baseline.json` (HEAD `9641256b5`; explain any difference with `git log 9641256b5..HEAD`):

| key | lines | key | lines |
|---|---:|---|---:|
| `go.total` | 127,962 | `web.total` | 106,009 |
| `go.chat` | 38,317 | `web.chat-surface` | 21,130 |
| `go.engine-provider` | 16,782 | `web.sidebar` | 18,562 |
| `go.engine-other` | 13,900 | `web.workspace-panes` | 14,211 |
| `go.service` | 10,849 | `web.editor` | 11,538 |
| `go.http` | 10,606 | `web.kit-ui` | 10,967 |
| `go.core` | 8,721 | `web.core` | 5,979 |
| `go.store` | 6,921 | `web.git-review` | 5,890 |
| `go.wiring` | 4,403 | `web.files` | 5,535 |
| `go.test-support` | 3,461 | `web.settings` | 5,407 |
| `go.stream` | 3,278 | `web.terminal` | 4,031 |
| `go.cmd` | 3,008 | `web.app-shell` | 2,150 |
| `go.mocks` | 2,658 | `web.extensions` | 609 |
| `go.dto` | 2,165 | | |
| `go.domain` | 2,025 | | |
| `go.app-misc` | 868 | | |

The Go module values sum to `go.total` and the web values to `web.total`; the script's own `total` is computed from files, so a mismatch means a file landed in two modules, which longest-prefix matching makes impossible; if you see one, stop and investigate.

- [ ] **Step 6: Prove growth, unassigned files and the update guard**

Run: `printf 'export const x = 1\n' > web/src/zz-probe.ts && node scripts/ratchet/loc-budgets.mjs; echo "exit=$?"; rm web/src/zz-probe.ts`
Expected: output starts `unassigned source files` and names `web/src/zz-probe.ts`; `exit=1`.

Run: `printf 'export const x = 1\n' > web/src/features/git/zz-probe.ts && node scripts/ratchet/loc-budgets.mjs --update; echo "exit=$?"; rm web/src/features/git/zz-probe.ts`
Expected: `grew   web.git-review: 5890 -> 5891`, `grew   web.total: 106009 -> 106010`, `refusing to raise a baseline`, `exit=1`, and `node scripts/ratchet/loc-budgets.mjs` after removing the probe is `ok` again (the baseline file was not touched).

- [ ] **Step 7: Commit**

```bash
git add scripts/ratchet/loc-budgets.mjs scripts/ratchet/loc-budgets.test.mjs scripts/ratchet/modules.json scripts/ratchet/loc-budgets-baseline.json
git commit -m "ci(ratchet): per-module source-line budgets for api and web, set to today's sizes"
```

---

### Task 4: Wire into make, CI and the conventions

**Files:**
- Modify: `Makefile` (`.PHONY` line, `pr-checks`, `ci`, new `ratchet` target)
- Modify: `.github/workflows/ci.yml` (new job after `validate-branch`)
- Modify: `CLAUDE.md` (Baselines bullet under "Lint suppressions")

**Interfaces:**
- Consumes: `scripts/ratchet/{file-size,counters,loc-budgets}.mjs` and `scripts/ratchet/*.test.mjs` from Tasks 1-3.
- Produces: `make ratchet` (runs the script tests, then the three gates; stops at the first failure), `make ratchet ARGS=--update` (rewrites baselines downward; `ARGS` is the variable `e2e` and `meter` already use), a required-looking CI job `ratchet-gates`.

- [ ] **Step 1: Add the Makefile target.** In `Makefile`: append `ratchet` to the `.PHONY` list; add

```make
RATCHETS := file-size counters loc-budgets

# File-size cap, pattern counters and per-module LOC budgets (scripts/ratchet/).
# Baselines only shrink: after deleting code run `make ratchet ARGS=--update`.
ratchet:
	@node --test scripts/ratchet/*.test.mjs
	@for g in $(RATCHETS); do node scripts/ratchet/$$g.mjs $(ARGS) || exit 1; done
```

and change `pr-checks: lint test-coverage build` to `pr-checks: ratchet lint test-coverage build` and `ci: lint test build` to `ci: ratchet lint test build` (cheap gate first, so a ratchet failure does not cost a coverage run).

- [ ] **Step 2: Verify the target**

Run: `make ratchet`
Expected: the node test summary with 0 failures, then three lines starting `file-size: ok`, `counters: ok`, `loc-budgets: ok`; exit 0.

Run: `make ratchet ARGS=--update`
Expected: three lines ending `already current`; `git status --short` shows no change under `scripts/ratchet/`.

Run: `make -n pr-checks | head -3`
Expected: the first recipe line is the `node --test` command.

- [ ] **Step 3: Add the CI job.** In `.github/workflows/ci.yml`, after `validate-branch`, add a job with no path filter (it reads only git-tracked text, takes seconds, and spans `api/` and `web/`):

```yaml
  ratchet-gates:
    if: github.event.pull_request.draft == false
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v7
      - name: Ratchet script tests
        run: node --test scripts/ratchet/*.test.mjs
      - name: File size (500-line cap; baseline may only shrink)
        if: ${{ !cancelled() }}
        run: node scripts/ratchet/file-size.mjs
      - name: Counters (timers, discarded errors, empty catch; baselines may only shrink)
        if: ${{ !cancelled() }}
        run: node scripts/ratchet/counters.mjs
      - name: LOC budgets (per module; budgets may only shrink)
        if: ${{ !cancelled() }}
        run: node scripts/ratchet/loc-budgets.mjs
```

The `!cancelled()` guards make one red run report every gate that failed, not only the first. No `setup-node`: `ubuntu-latest` ships Node >= 20, and the scripts use only `node:test`, `node:fs`, `node:child_process`, `node:path`, `node:url`.

- [ ] **Step 4: Validate the workflow file**

Run: `grep -n '^  ratchet-gates:' .github/workflows/ci.yml`
Expected: one line. Then read `git diff .github/workflows/ci.yml` and confirm indentation matches the neighbouring jobs (2-space job keys, 6-space steps) and that no existing job changed.

- [ ] **Step 5: Update the convention.** In `CLAUDE.md`, replace the Baselines bullet with:

```
- Baselines (`api/deadcode-baseline.txt`, `web/knip.json` ignoreIssues, the
  `*_BASELINE` lists in `web/eslint.config.js`, golangci-lint's `new-from-rev`,
  `scripts/ratchet/*-baseline.json`) only shrink: fix an entry, delete it; never
  add one. `make ratchet` (in `make pr-checks`) gates file size, timer/`_ =`/
  empty-catch counts and per-module LOC; after deleting code run
  `make ratchet ARGS=--update` and commit the lower numbers.
```

- [ ] **Step 6: Final sweep**

Run: `make ratchet`
Expected: exit 0 as in Step 2.

Run: `git status --short`
Expected: only `Makefile`, `.github/workflows/ci.yml`, `CLAUDE.md` modified; no stray `.bak`, `zz-probe` or temp files anywhere.

- [ ] **Step 7: Commit**

```bash
git add Makefile .github/workflows/ci.yml CLAUDE.md
git commit -m "ci(ratchet): run the ratchet gates in make pr-checks and as a CI job"
```

---

## How later stages use this

- Every stage ends with `make ratchet ARGS=--update` and commits the lower numbers; the stage review diffs `scripts/ratchet/*-baseline.json` and rejects any number that went up.
- Moving code between modules, or creating a module (S1 ledger, S4 `core`, S7 flattening): edit `scripts/ratchet/modules.json`, move the budget between the two module numbers by hand in the same commit, run the check; `<lang>.total` must not rise. Re-keying an oversize file in `file-size-baseline.json` is the only other allowed hand edit (key changes, value unchanged or lower).
- Concurrent stage branches that both lowered a baseline will conflict on the numbers: resolve by taking either side and re-running `make ratchet ARGS=--update`, never by hand-merging values.
