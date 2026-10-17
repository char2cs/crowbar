# S6 Terminal: Wrapped Links Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A URL or file path that the pane width soft-wrapped is one clickable link across every row it spans (live view, after re-attach, in scrollback). A line the program hard-broke is never joined.

**Architecture:** The daemon's screen model gains a per-row wrap bit (xterm's `isWrapped` meaning). The serializer and the diff emitter write a wrapped row at full width and continue the next row with no cursor move, so the client xterm autowraps and sets its own `isWrapped`. One client tokenizer reads the logical line through `isWrapped`; it replaces both link providers.

**Tech Stack:** Go (`api/internal/core/terminal/internal/model`, charmbracelet x/vt), TypeScript (xterm.js 6 `IBufferLine.isWrapped`), vitest, Tauri (`make dev-desktop`).

**Spec:** `docs/plans/2026-10-16-crowbar-architecture.md` §11 row S6 ("wrap bit in the screen model; one link tokenizer"; deletes the second link provider and the "accepted residual"; gate: conformance case for wrapped links), §7 row "Wrapped terminal links cut in half", §8 ("Merge: two terminal link providers into one"; "Keep as is: the terminal screen model and its conformance suite (add the wrap bit)").

## Global Constraints

- Worktree: `/Users/char2cs/.crowbar/projects/cb81ea03-54b7-4093-8bdd-fdd6b183e91d/github.com/char2cs/crowbar/fix/refactor-chats/worktree`, branch `fix/refactor-chats`. Repo `CLAUDE.md` is binding. Never push; no PRs; never bare `git stash`.
- No flags, no parallel old/new paths, no shims: each task deletes what it replaces in the same change. Ships once with the branch.
- A fix starts with a failing test. No timers, polls, retries, extra flags or resync effects. No sleeps in tests (channels, `waitFor`, the `write(data, cb)` callback).
- Go: `-tags noEmbed`; `make lint` (new code) and `make deadcode` before the last commit of a task. Files stay ≤ 500 lines; a file already over 500 may not grow. Early returns, wrapped errors, comments ≤ 3 lines of *why*.
- Web: kebab-case files; tests under `web/src/__tests__/` mirroring `src/`; `@/` imports; ≤ 3 `vi.mock` per file; run ONLY targeted vitest files (`node_modules/.bin/vitest run <file>`), never the full suite; typecheck with `bun tsc --noEmit` (never `bunx tsc`). `knip.json` ignoreIssues and eslint baselines only shrink.
- Disk is nearly full: `df -h /System/Volumes/Data` before any build; under 8 GB free, stop and report. This worktree has no `web/node_modules` and no `desktop/src-tauri/target` yet.
- Never touch `~/.crowbar` or its socket; dev runs use `CROWBAR_HOME=<worktree>/.crowbar` (root `Makefile:11`). Never read `~/.claude` / `~/.codex`.
- Commit trailer on every commit: `Co-Authored-By: Claude Sonnet 5.5 <noreply@anthropic.com>`.

## Review Focus

Failure modes the spec implies but a happy-path test would miss. Each is pinned in the task named.

1. A URL ends exactly on the last column and the program prints CRLF; the next row starts a different token. Must stay two links (T1 Go + web, T4, T5, T6).
2. A chain whose head scrolls into history in one frame and whose tail in the next. Must stay joined in the client's scrollback (T5).
3. Pane resize after wrapping: the wrap bit is dropped on any width change, nothing panics, nothing stale is joined (T3, T7).
4. App turns autowrap off (`?7l`), or an alt-screen TUI repaints with CUP+EL2: no flags appear, no extra bytes are emitted (T3, T5).
5. Wide glyphs before the URL: link ranges land on the right cells (T6).

## Where it lives today

| Piece | Location |
|---|---|
| Screen model (Go) | `api/internal/core/terminal/internal/model/`: `vtModel` `vt_model.go:39`; emulator seam `vt_emu.go:18-51`, `ScrollbackLine` `vt_emu.go:130`; emulator pin `api/go.mod:10` (`x/vt v0.0.0-20260629091435-9c70f75e26a4`) |
| Why there is no bit | x/vt keeps pending-wrap in private `atPhantom` (`utf8.go:54-60,91`, `cc.go:26-36`); `Scrollback.Push` trims trailing cells (`scrollback.go:29-55`). Newer upstream (`x/vt v0.1.0`, 2026-10-12) and `vito/midterm v0.2.5` also expose no per-row bit |
| Writers that hard-break | serializer `vt_serializer.go:85-108` (`\r\n` after every row, trailing trim); diff `diff.go:398-418` (`writeScreenDiff`: CUP + EL per dirty row) and `diff.go:367-393` (`writeScrollbackDelta`); `encodeLine` `vt_serializer.go:229` |
| "Accepted residual" | `model.go:34-51` (package doc), `UPSTREAM.md:69-98` (D1) and `:155-162` (checklist item), `vt_serializer_test.go:568-600` (`TestSoftWrapResidualHardBreaksButPreservesCells`) |
| Conformance suite | `conformance_test.go:48-70` (`gridString`), `:72-86` (`conformanceStep`), `:377` (randomized splits); `diff_test.go:31` (`scrollbackStrings`) |
| Link provider 1 (files, one row) | `web/src/features/terminal/lib/terminal-file-links.ts:96-126` (reads one row via `translateToString`, `:102-109`) |
| Link provider 2 (URLs) | `web/src/features/terminal/hooks/use-terminal-addons.ts:29-39` (`WebLinksAddon`), loaded at `mount-xterm.ts:61`; dependency `web/package.json:60`; knip entry `web/knip.json:162` |
| Sibling heuristic (stays) | `web/src/features/terminal/utils/selection-text.ts:1-28` infers wraps from the last cell for copy. Out of scope here |

Root cause in one line: the daemon addresses every row with CUP, so the client xterm never autowraps and `isWrapped` is false on every row; the addon already joins wrapped rows, it just never sees any.

## Rulings

1. **Bit source:** patch x/vt in-tree. A nested module `api/third_party/x-vt` (verbatim copy of the pin, then a small patch) wired by `replace` in `api/go.mod`; delete it when upstream exposes the bit. Rejected: inferring wrap from cursor callbacks (a wrap and `LF`+`CHA` are indistinguishable; bottom-row scroll fires no callback), feeding the emulator byte by byte (re-implements the parser), swapping to midterm (same gap, rewrites three adapters), a client-side side channel (second copy of state).
2. **Convention:** the flag lives on the continuation row, as xterm's `isWrapped`. `RowWrapped(y)` / `ScrollbackRowWrapped(i)`.
3. **Lifecycle:** set only by autowrap overflow; moves with its row; cleared by an erase that covers a whole row from column 0 and by any width change (grid and scrollback; no reflow); row 0 is never flagged unless the line above it is in the scrollback ring.
4. **Wire contract:** a continuation is never separated from its head. Heads are written at full width untrimmed; the next row follows with no CUP, CR or LF. The diff unit is the chain. No new keyframe triggers.
5. **Client:** one tokenizer in `terminal-links.ts` over the logical line (walk `isWrapped`, ≤ 2048 chars each way). `@xterm/addon-web-links` and its dependency are deleted.
6. **Deleted residual:** `model.go:34-51`, UPSTREAM.md D1 and its checklist item, the old residual test. D1a (panic-recovery history loss) and D2 (xterm oracle) stay; D1a's "resolves WITH D1" wording becomes standalone.

## Red → green map

| Red in T1 | Turns green in |
|---|---|
| `TestSerialize_SoftWrappedURLIsUnbroken` | T4 |
| `TestDiffEmitter_SoftWrappedURLWrittenUnbroken`, `TestRegression_TerminalSession_WrappedLinesStayUnbroken` | T5 (black-box needs T4 for re-attach and T5 for live frames) |
| web `wrapped URL` / `hard-broken` provider tests | T6 |
| Guards that pass today and must never go red: `TestSerialize_ProgramHardBreakStaysBroken`, `TestDiffEmitter_ProgramHardBreakNotJoined`, the xterm-idiom tests | n/a |

---

### Task 1: Red cases for wrapped vs hard-broken lines

**Goal:** Pin the wire contract and the user symptom before any code changes. One URL soft-wrapped across two rows must reach the client unbroken; the same bytes hard-broken by the program (exact-fit row, then CRLF) must not be joined.

**Files:**
- Modify: `api/internal/core/terminal/internal/model/conformance_test.go` (add `const wrapTestURL = "https://example.com/docs/guide/getting-started?ref=crowbar"`, 58 chars)
- Modify: `api/internal/core/terminal/internal/model/vt_serializer_test.go` (replace `TestSoftWrapResidualHardBreaksButPreservesCells`, `:568-600`; the file may not grow)
- Modify: `api/internal/core/terminal/internal/model/diff_test.go`
- Create: `api/tests/integration/terminal/wrapped_lines_test.go` (new file: `terminal_test.go` is already 1015 lines and must not grow)
- Modify: `web/src/__tests__/features/terminal/lib/terminal-file-links.test.ts`

**Interfaces:**
- Produces: `wrapTestURL`; the test names in the map above. At 40 columns `"see " + wrapTestURL` fills row 1 with `"see " + url[:36]` and row 2 with `url[36:]` (22 chars).

- [ ] **Step 1: Write the Go cases** (`New(40, 6, 100)`).
  - `TestSerialize_SoftWrappedURLIsUnbroken`: `m.Write("see " + wrapTestURL)`; `assert.Contains(string(s.Serialize(m)), wrapTestURL)`.
  - `TestSerialize_ProgramHardBreakStaysBroken`: `m.Write("see " + url[:36] + "\r\n" + url[36:])` (row 1 is exactly 40 wide); payload contains `url[:36] + "\r\n" + url[36:]` and does not contain `wrapTestURL`.
  - `TestDiffEmitter_SoftWrappedURLWrittenUnbroken` / `TestDiffEmitter_ProgramHardBreakNotJoined`: prime an emitter on the blank model, write the same bytes, `Emit`; the frame contains / does not contain `wrapTestURL` contiguous.
- [ ] **Step 2: Black-box regression** `(s *TerminalSuite) TestRegression_TerminalSession_WrappedLinesStayUnbroken` (build tag `integration`, uses `newSessionWithLiveClient`, 80x24). Drive `printf 'https://example.com/%0100d\n' 0 | tr 0 a; printf '%080d\n' 0 | tr 0 x; echo yyyyyyyyyy; M=WRAP_DONE; echo "${M}_7d1c"; sleep 30` (the command echo never contains the output text). Wait on the marker in the live frames, then assert on the accumulated live frames AND on the first frame of a second client (the re-attach redraw): it contains `"https://example.com/" + strings.Repeat("a", 100)` contiguous, and contains `strings.Repeat("x", 80) + "\r\nyyyyyyyyyy"` (exact-fit hard break stays broken). End with the existing kill + "ended" wait.
- [ ] **Step 3: Web cases** in `terminal-file-links.test.ts`. Helper: `new Terminal({ cols: 40, rows: 6, allowProposedApi: true })`, `await new Promise<void>((r) => t.write(data, r))`, capture the provider with `vi.spyOn(t, 'registerLinkProvider')` after `registerTerminalFileLinks(t, { getRoot, openFile })`, call `provideLinks(row, cb)`.
  - `joins a URL the pane width wrapped`: data `"see " + url`; `provideLinks(2)` and `provideLinks(1)` each return one link with `text === url`, `range` start `{x: 5, y: 1}`, end `{x: 22, y: 2}`.
  - `does not join a program hard-break`: data `"see " + url.slice(0, 36) + "\r\n" + url.slice(36)`; `provideLinks(1)` returns one link with `text === url.slice(0, 36)` and a one-row range; `provideLinks(2)` returns `undefined`.
  - `xterm wrap idioms the daemon emitter relies on` (guards, pass today): an unbroken full-width write then the next row sets `getLine(1)?.isWrapped === true`; `CUP(2,1)` + `EL0` clears it; `CUP(2,1)` + cells with no EL leaves it set; scrolling a flagged row into scrollback keeps the flag.
- [ ] **Step 4: Run, confirm red for the right reason**
  - `cd api && go test -tags noEmbed ./internal/core/terminal/internal/model -run 'TestSerialize_(SoftWrapped|ProgramHard)|TestDiffEmitter_(SoftWrapped|ProgramHard)' -count=1` → the two `SoftWrapped*` fail on the missing contiguous URL, the two hard-break guards pass.
  - `cd api && go test -tags 'integration noEmbed' ./tests/integration/terminal -run 'TestTerminalSuite/TestRegression_TerminalSession_WrappedLinesStayUnbroken' -count=1` → fails on the contiguous-URL assertion.
  - `cd web && bun install --frozen-lockfile && node_modules/.bin/vitest run src/__tests__/features/terminal/lib/terminal-file-links.test.ts` → the two provider cases fail (no URL link), the idiom guards pass.
- [ ] **Step 5: Commit** (red by design; record the failing test names in the body)

```bash
git add api/internal/core/terminal/internal/model api/tests/integration/terminal/wrapped_lines_test.go web/src/__tests__/features/terminal/lib/terminal-file-links.test.ts
git commit -m "test(terminal): pin the wire contract for soft-wrapped vs hard-broken lines (red)"
```

---

### Task 2: Vendor x/vt verbatim

**Goal:** A reviewable, behaviour-neutral base for the patch: the pinned x/vt source in-tree, wired by `replace`.

**Files:**
- Create: `api/third_party/x-vt/` (all non-test `*.go`, `go.mod`, `LICENSE` of the pin; no tests, no `go.sum`)
- Modify: `api/go.mod` (add `replace github.com/charmbracelet/x/vt => ./third_party/x-vt`, keep the `require` line), `api/go.sum` (only removals)

- [ ] **Step 1: Copy.** `mkdir -p api/third_party/x-vt && cp "$(go env GOMODCACHE)/github.com/charmbracelet/x/vt@v0.0.0-20260629091435-9c70f75e26a4"/{*.go,go.mod,LICENSE} api/third_party/x-vt/ && rm api/third_party/x-vt/*_test.go && chmod -R u+w api/third_party/x-vt` (module-cache files are read-only).
- [ ] **Step 2: Wire.** Add the `replace`; run `cd api && GOFLAGS=-mod=mod GOPROXY=off go mod tidy`. If it needs the network, stop and report.
- [ ] **Step 3: Verify neutrality.** `diff -r -x '*_test.go' -x go.sum -x testdata "$(go env GOMODCACHE)/github.com/charmbracelet/x/vt@v0.0.0-20260629091435-9c70f75e26a4" api/third_party/x-vt` prints nothing. `cd api && go build -tags noEmbed ./... && go test -tags noEmbed ./internal/core/terminal/internal/model -count=1` is green (T1's reds excepted). `make lint` and `make deadcode` report nothing new (the nested module is outside `./...`; if `deadcode-baseline.txt` changes, the replace is wrong).
- [ ] **Step 4: Commit**

```bash
git add api/third_party/x-vt api/go.mod api/go.sum
git commit -m "build(api): vendor charmbracelet/x/vt at the pinned commit"
```

---

### Task 3: The model owns a per-row wrap bit

**Goal:** `RowWrapped` / `ScrollbackRowWrapped` report xterm-`isWrapped` semantics for the grid and the scrollback; the "accepted residual" stops being true and is removed from the docs.

**Files:**
- Modify (patch): `api/third_party/x-vt/{utf8.go,screen.go,scrollback.go,emulator.go}`
- Modify: `api/internal/core/terminal/internal/model/vt_emu.go` (interface at `:18-51`), `vt_model_test.go` (`fakeEmu` at `:43` gets the two methods)
- Create: `api/internal/core/terminal/internal/model/vt_emu_test.go`

**Interfaces:**
- Produces on `*vt.Emulator`: `RowWrapped(y int) bool` (current screen, row y continues row y-1), `ScrollbackRowWrapped(i int) bool` (i = 0 oldest). On `*vt.Scrollback`: `PushWrapped(line uv.Line, wrapped bool)` (replaces `PushN`, which becomes dead). Seam: `emulator.RowWrapped`, `emulator.ScrollbackRowWrapped` (promoted by `vtEmu`, no body needed).

- [ ] **Step 1: Write `TestRowWrapped`** (table, 10x4 model via `newVTModel(10, 4, 100)`; read `vm.emu.RowWrapped(y)` for y in 0..3):

| case | bytes | grid flags | scrollback flags |
|---|---|---|---|
| autowrap sets it | `0123456789ABCDEFGHIJ` | F T F F | |
| exact fit + CRLF is a hard break | `0123456789\r\nABCDEFGHIJ` | F F F F | |
| autowrap off | `\x1b[?7l0123456789ABCDEFGHIJ` | F F F F | |
| EL0 at col 0 clears | wrap, then `\x1b[2;1H\x1b[K` | F F F F | |
| EL0 mid-row keeps | wrap, then `\x1b[2;5H\x1b[K` | F T F F | |
| ED2 clears | wrap, then `\x1b[2J` | F F F F | |
| flag travels with its row; orphan keeps it when scrollback holds the head | wrap, then `\r\n\r\n\r\nX` | T F F F | F |
| scrollback carries flags | 30 chars, then `\r\n` x4 | F F F F | F T T |
| alt screen never keeps an orphan row 0 | `\x1b[?1049h`, 20 chars, `\r\n` x4 | F F F F | |
| width change drops all | 30 chars, `\r\n` x3 (grid T F F F, scrollback F T), then `Resize(12, 4)` | F F F F | F F |
| height-only change keeps | same, then `Resize(10, 6)` | T F F F F F | F T |

- [ ] **Step 2: Run to fail** (`undefined: RowWrapped`): `cd api && go test -tags noEmbed ./internal/core/terminal/internal/model -run TestRowWrapped -count=1`.
- [ ] **Step 3: Patch x/vt** (keep the diff small, comments ≤ 3 lines):
  - `Screen.wrapped []bool` (len = height) beside the buffer; `Scrollback.wrapped []bool` parallel to `lines`, mirrored in eviction (`scrollback.go:49-53`), `SetMaxLines`, `Clear`.
  - `utf8.go:54-60`: after `e.index()` resolves the new cursor row, mark that row wrapped.
  - `Screen.InsertLine/DeleteLine` (`screen.go` ~`:316-372`): when the region spans the full width, shift flags exactly as the lines move, new rows false; `DeleteLine` pushes flags with each line via `PushWrapped`; if the discarded row 0 did not go to the emulator-visible (main-screen) scrollback, clear the new row 0. The alt screen has a private scrollback nothing reads, so it counts as "no history"; so does a margin-confined region.
  - `ClearArea` / `FillArea` (`:120-135`): a row fully covered from column 0 loses its flag. `Reset`, `ClearWithScrollback` (`:94-108`, use `PushWrapped`), `Resize` (`:74-83`: any width change clears grid and scrollback flags; height change grows/truncates the slice).
  - `emulator.go`: the two accessors, reading `e.scr` and the main screen's scrollback.
- [ ] **Step 4: Seam.** Add the two methods to `emulator` (`vt_emu.go`) and to `fakeEmu`.
- [ ] **Step 5: Run to pass.** `go test -tags noEmbed ./internal/core/terminal/internal/model -run 'TestRowWrapped' -count=1`, then the whole `model` package: only T1's reds fail.
- [ ] **Step 6: Commit** `feat(terminal): the screen model owns a per-row wrap bit` (patch to `api/third_party/x-vt` and model seam in one commit so the diff of the third-party dir is exactly the patch).

---

### Task 4: The serializer writes wrapped rows unbroken

**Goal:** Keyframes (attach, resync, restore) carry the wrap bit to the client; the residual is deleted.

**Files:**
- Modify: `api/internal/core/terminal/internal/model/vt_serializer.go` (`writeContent`, `:85-108`)
- Create: `api/internal/core/terminal/internal/model/wrap.go`, `wrap_test.go`
- Modify: `model.go` (delete the residual paragraph `:34-51`), `UPSTREAM.md` (replace D1 `:69-98` with a short "Patched copy" record: what is patched, delete it when a tagged upstream exposes a per-row bit; delete the D1 checklist item `:155-162`; make D1a's revisit trigger standalone)

**Interfaces:**
- Produces in `wrap.go`: `padToCols(cells []uv.Cell, cols int) []uv.Cell` (extends with `uv.EmptyCell`; a scrollback head may have been trimmed, so an unbroken head must be padded to reach the last column).

- [ ] **Step 1: Failing tests in `wrap_test.go`.**
  - `TestSerialize_RoundTripPreservesWrapFlags`: feed a model a soft-wrapped URL, an exact-fit hard break, a 3-row chain, and enough output to push a head to scrollback while its tail is still on screen; `Serialize`; write the payload into a fresh `New(...)`; flags of every grid row and scrollback line equal the source model's (includes the head/tail straddle at the scrollback/grid boundary).
  - `TestPadToCols` (table).
  - A wrapped head whose last stored cell is a printed space (trimmed in scrollback) still round-trips joined.
- [ ] **Step 2: Run to fail**; T1's `TestSerialize_SoftWrappedURLIsUnbroken` is still red.
- [ ] **Step 3: Implement `writeContent` rules.** For scrollback line `i` and grid row `y`, "next continues" is `ScrollbackRowWrapped(i+1)` (for the last scrollback line: `RowWrapped(0)`) / `RowWrapped(y+1)`. When the next row continues: pad to `cols`, encode untrimmed, and emit no separator. Otherwise: encode trimmed and emit `\r\n` as today. A continuation row always emits at least one cell (a space when blank) so the client wraps onto it. Delete the residual comment at `model.go:34-51` and update `UPSTREAM.md` as listed.
- [ ] **Step 4: Run to pass:** the `model` package (T1's diff reds excepted), including the pre-existing `TestRoundTripThroughXVT` and `vt_serializer_reattach_test.go`.
- [ ] **Step 5: Commit** `feat(terminal): serializer writes soft-wrapped rows unbroken`.

---

### Task 5: The diff emitter keeps chains unbroken, including across scrollback

**Goal:** Live frames carry the wrap bit; the client's flags equal the model's after every frame, including a chain straddling the scrollback boundary. The conformance suite proves it.

**Files:**
- Create: `diff_chrome.go`, `diff_chrome_test.go` (pure move of `chromeBase`, `captureChrome*`, `copyModes`, `recaptureChrome`, `colorsEqual`, `writeChromeDelta`, `writeModeDelta`, `writeCursorChromeDelta`, `modesEqual` out of `diff.go`, with their `TestDiffEmitter_*Forwarded` tests; `diff.go` is 640 lines and must end ≤ 500)
- Modify: `diff.go`, `wrap.go`, `wrap_test.go`, `diff_test.go`, `conformance_test.go`, `diff_alloc_test.go` (all under `api/internal/core/terminal/internal/model/`)

**Interfaces:**
- Consumes: `padToCols` (T4), `emulator.RowWrapped`, `emulator.ScrollbackRowWrapped` (T3).
- Produces: `chainEnd(emu emulator, y, rows int) int` in `wrap.go` (last grid row of the chain starting at row `y`, via `emu.RowWrapped(y+1)`; `TestChainEnd` table in `wrap_test.go`); `DiffEmitter.lastWrapped []bool` (per grid row, what the client holds; set in `Prime`, reused in place each `Emit`); test helper `wrapFlags(m TerminalModel) string` (grid then scrollback flags as text).

- [ ] **Step 1: Mechanical split.** Move the chrome code and its tests into `diff_chrome*.go`. `go test ... -run TestDiffEmitter -count=1` unchanged. Commit `refactor(terminal): split diff chrome out of diff.go`.
- [ ] **Step 2: Lift the cases into the sacred suite (red).** In `conformanceStep` add `require.Equal(t, wrapFlags(m), wrapFlags(sim.m), ...)` after the grid equality. Add scripted scenarios (20x6): soft-wrapped URL; hard break then rewrite as soft wrap with identical cells (flag-only flip, must emit); soft wrap then `\x1b[2;1H\x1b[2K` retype hard; 3-row chain; app with `?7l` rewriting a flagged row. Add `TestConformance_WrappedChainsUnderScroll` (20x6, one wrapped line of 25-45 chars per chunk, 30 chunks, diff path asserted via `needKeyframe == false`, also `scrollbackStrings` and flags equal) and `TestConformance_WrappedChainStraddlesScrollback` (head commits in one `Emit`, tail in the next). The existing `TestConformance_RandomizedByteSplits` now also exercises wraps through the extended check. Run: reds on the new scenarios and the randomized test.
- [ ] **Step 3: Screen diff in chains** (`writeScreenDiff`, `diff.go:398-418`). A row is dirty when `!rowEqualsGrid` or `RowWrapped(y) != lastWrapped[y]`. Emit per chain `[y0..chainEnd]` when any row in it is dirty, ascending: `CUP(y0+1,1)`; `EL0` first unless the chain starts at row 0 and `RowWrapped(0)` (the client row keeps the flag it holds); head rows padded to `cols` and untrimmed; continuation rows with no CUP; the tail trimmed then `EL0`. A chain of one row is today's `CUP + EL + trimmed row`. If the client's autowrap is off (`modes[7]` present and false; absent means on, as in `vt_serializer.go:127`) bracket the chain with `?7h` / `?7l`. Record `lastWrapped` as emitted.
- [ ] **Step 4: Scrollback delta continuity** (`writeScrollbackDelta`, `diff.go:367-393`). Advance by the batch actually written. Shrink a batch until the line after it is not a continuation (keep ≥ 1). A line that continues the previous one in the batch is written with no CUP/EL after a padded head. The first line of the delta, when it is a continuation, is written `CUP(1,1)` + cells with no leading `EL` only if `lastWrapped[0]` says the client row already holds the flag, else as a plain row. When `RowWrapped(0)` (the viewport top continues the last committed line), the final batch is limited to `rows-1` and the viewport row 0 content is written contiguously after the last committed line before parking, so exactly the committed lines scroll out and the continuation stays on the client with its flag; set `lastWrapped[0]` accordingly. No new keyframe trigger; update nothing in `Emit`'s trigger list.
- [ ] **Step 5: Run to pass.** `go test -tags noEmbed -race ./internal/core/terminal/... -count=1`; the black-box `TestRegression_TerminalSession_WrappedLinesStayUnbroken` from T1 is green; `diff_alloc_test.go` gains one `testing.AllocsPerRun` case: a no-change `Emit` on a primed model holding a chain allocates no more than the same case without a chain. `make lint`, `make deadcode`.
- [ ] **Step 6: Commit** `feat(terminal): diff emitter keeps soft-wrapped chains unbroken`.

---

### Task 6: One link tokenizer over the logical line

**Goal:** One provider turns URLs and file paths into links across wrapped rows; the addon, its dependency and the second provider are gone.

**Files:**
- Rename: `web/src/features/terminal/lib/terminal-file-links.ts` → `terminal-links.ts`; `web/src/__tests__/features/terminal/lib/terminal-file-links.test.ts` → `terminal-links.test.ts` (use `git mv`, mechanical first commit)
- Modify: `web/src/features/terminal/lib/mount-xterm.ts` (`:7,22,61-63`), `web/src/features/terminal/hooks/use-xterm-instance.ts` (`:13,22-48,153`), `web/src/features/terminal/hooks/use-terminal-addons.ts` (delete `loadWebLinksAddon`, `:1,6,29-39`), `web/src/features/terminal/styles/terminal.css:89` (comment names the addon; say "xterm's link layer"), `web/src/__tests__/features/terminal/lib/mount-xterm.test.ts` (dep key `fileLinks` → `links`, add `openUrl: vi.fn()`), `web/package.json:60` and `web/bun.lock` (`bun remove @xterm/addon-web-links`), `web/knip.json:162` (delete the entry; if knip still flags an export, remove it, never re-add)

**Interfaces:**
- Produces: `TerminalLinksOptions { getRoot: () => string | undefined; openFile: (absolutePath: string, line?: number) => void; openUrl: (uri: string) => void; onUnresolved?: (candidateText: string) => void }`; `type LinkToken = { kind: 'url'; text: string; start: number; end: number } | { kind: 'file'; text: string; path: string; line?: number; start: number; end: number }` (`end` exclusive, indices into the logical text); `tokenizeLinks(text: string): LinkToken[]` (URL tokens win; a file candidate overlapping a URL token is dropped, which replaces `URL_PREFIX_RE`); `registerTerminalLinks(terminal: Pick<Terminal, 'buffer' | 'registerLinkProvider'>, options: TerminalLinksOptions): IDisposable`. `resolveFileLinkPath` and `workspaceRelativePath` keep their signatures. Mount: `mountXterm(container, options, { write, links })`.

- [ ] **Step 1: Mechanical rename** (identifiers, imports, test file); targeted vitest green; commit `refactor(web): rename terminal file links to terminal links`.
- [ ] **Step 2: Failing tests** (extend `terminal-links.test.ts`; T1's provider cases now call `registerTerminalLinks`):
  - tokenizer: `see https://x.dev/a.` → URL without the trailing period; `(https://x.dev/a)` → no brackets; `ftp://x` and `www.x.dev` → none; `web/src/a.ts:12` inside `https://github.com/a/b/web/src/a.ts` → one URL token only; the existing file-path cases ported from `parseFileLinkCandidates` (delete that function and `URL_PREFIX_RE`).
  - provider over real xterm: wrapped URL joins (T1 numbers); 3-row chain returns the same link from rows 1, 2 and 3 and `undefined` from an unrelated row; a hard break at the exact last column is not joined (Review Focus 1); a wrapped file path `web/src/features/terminal/lib/terminal-links.ts:12` at 30 columns opens with `line === 12`; wide glyphs before the URL (`日本語 ` + url) give start `{x: 8, ...}` (3 wide glyphs = 6 cells + space); `activate` on a URL calls `openUrl(url)` once; a chain longer than 2048 chars still returns without walking the whole buffer.
- [ ] **Step 3: Implement.**
  - Logical line: from the hovered 0-based row walk up while the row `isWrapped`, down while the next row `isWrapped`, ≤ 2048 chars each direction. Build text per cell via `getCell`: skip width-0 cells, map `''` to `' '` only when a later cell in that row has content (so end-of-content and early-wrap gaps vanish), and keep `cellOf[unit] = {x, y, width}` per UTF-16 unit for range mapping. Range is xterm's 1-based inclusive; return only links intersecting the hovered row.
  - URL acceptance ported from `@xterm/addon-web-links@0.12.0` (`src/WebLinksAddon.ts`, `src/WebLinkProvider.ts`): regex `/(https?|HTTPS?):[/]{2}[^\s"'!*(){}|\\\^<>`]*[^\s"':,.!?{}|\\\^~\[\]`()<>]/g` plus its `isUrl` host-prefix check via `new URL`. Read the installed package before `bun remove`.
  - Add `openUrl` to `TerminalLinksOptions` now and pass `openUrl: vi.fn()` at every test call site.
  - `use-xterm-instance.ts`: `linksFor` adds `openUrl: (uri) => { openExternalUrl(uri).catch((error) => console.error('Failed to open link:', error)) }` (behaviour moved unchanged from the addon wiring; no confirmation step, as before).
- [ ] **Step 4: Run.** Targeted vitest: `terminal-links.test.ts`, `mount-xterm.test.ts`, `terminal.test.tsx`; `bun tsc --noEmit`; `bunx eslint <changed files> --max-warnings 0`; `bunx prettier --check <changed files>`; `bun run knip` (entry deleted, nothing new). Confirm `grep -rn "addon-web-links\|loadWebLinksAddon\|parseFileLinkCandidates\|URL_PREFIX_RE" web/src web/package.json` prints nothing.
- [ ] **Step 5: Commit** `feat(web): one terminal link tokenizer over the logical line`.

---

### Task 7: Live check in Tauri

**Goal:** Prove the symptom is gone in the real app, on the real daemon, in the isolated home.

**Files:** none (nothing is committed; no QA log goes in the repo).

- [ ] **Step 1: Preconditions.** `df -h /System/Volumes/Data` ≥ 8 GB or stop and report. Reuse a running dev instance of this worktree if one exists (never start a second). From the repo root: `caffeinate -u -t 900 &` (rAF is suspended on a sleeping display), then `make dev-desktop` (isolated `CROWBAR_HOME=<worktree>/.crowbar`; the first build fetches the sidecar from local source, so the patched model is in it; cold Rust build is large).
- [ ] **Step 2: Drive with the Tauri MCP only** (no osascript, no screen capture). Traps: xterm's GPU layer is absent from screenshots, so assert through the DOM and IPC, never pixels; `webview_keyboard` does not reach xterm, so produce output by feeding the PTY through the daemon's terminal WebSocket from a throwaway script against the dev unix socket (framing as in `api/tests/kit` `DialTerminalPTY`/`ParseTerminalFrame`), never touching `~/.crowbar`; `webview_execute_js` cannot await (stash on `window.__x`, read in a second call).
- [ ] **Step 3: Scenarios** (hover = pointer-move over the cell; link hover sets `xterm-cursor-pointer` on `.xterm`, `terminal.css:~100`; click is observed as the shell-open IPC payload via the Tauri IPC monitor):
  1. A 110-char URL printed in an 80-col pane: hover row 1 and row 2 both show the pointer; click opens the full URL.
  2. Exact-fit hard-broken pair (80 `x`, newline, `yyyyyyyyyy`): no pointer join; nothing opens a partial URL.
  3. A file path wrapped by a narrow pane: click opens the file in the editor tab at the line.
  4. URL, then 60 lines of output; scroll back; hover both rows of the URL (and once with the head scrolled off and the tail at the viewport top).
  5. Switch workspace away and back (re-attach keyframe), then drag the sash narrower and wider: scenario 1 still links, no console errors, no stuck blank rows.
  6. Stream `yes "$(printf 'https://example.com/%0100d' 0 | tr 0 a)" | head -3000`: scroll position and CPU stay sane (a keyframe storm would reset scroll and spike the daemon).
- [ ] **Step 4: Last gates.** `cd api && go test -tags noEmbed -race ./internal/core/terminal/... -count=1 && make lint && make deadcode`; web targeted files, `bun tsc --noEmit`, `bun run knip`. Record the outcome of each scenario in the final report (not in the repo). If any scenario fails, write the failing test first (Task 1 style) and fix at its owner; do not patch with a timer or a resync.

---

## Out of scope (tracked, not planned)

- `selection-text.ts` keeps its edge heuristic until a follow-up retires it in favour of xterm's native `getSelection()` now that `isWrapped` is real; it is not in S6's Deletes column and changes copy behaviour.
- Reflow on width change (the bit is dropped; the next `Emit` keyframes), a wide glyph straddling the last column (x/vt does not early-wrap), joining wrapped rows in `ScreenText()`.
- Upstreaming `RowWrapped` to charmbracelet/x so `api/third_party/x-vt` can be deleted.
