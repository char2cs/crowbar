# Diff Perf Phase 2 — Windowed Diff API — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Serve the branch diff as an outline plus per-file patch text, so the client can render a 1M-line diff without ever holding it — and back find-in-diff with a server-side search.

**Architecture:** Three read endpoints on top of Phase 1's `exec.GitStream`. `/review/outline` streams the diff and keeps only `@@` headers (O(hunks)). `/review/patch?path=` returns one file's unified patch as `text/plain`. `/review/search` streams and scans, returning file+line hits. Nothing here changes the existing `/review` or `/review/files`; Phase 3 switches the frontend over and deletes the old payload.

**Tech Stack:** Go 1.x + gin, `exec.GitStream` (Phase 1).

## Global Constraints

- Spec: `docs/superpowers/specs/2026-07-27-diff-subsystem-at-scale-design.md`, "Layer 2/3" and "Diff search". Phase 2 only.
- **Invariant 3 governs every endpoint here:** none may buffer the diff. Use `exec.GitStream`; never `exec.Git` on a diff-sized command. A test that only checks output correctness will pass on a buffering implementation — prove memory behaviour explicitly.
- Nothing on a recurring timer. These are user-initiated reads.
- Additive only. Do not modify `/review`, `/review/files`, or any frontend file.
- Multiple sessions share this worktree's git index. **Subagents must NOT run any git write command.** Leave changes in the working tree.
- Do not touch: `api/internal/app/usecases/workspace/workspace.go`, `api/tests/integration/files/home_files_test.go`, or any `web/` file.
- No timing-based synchronization in tests.
- `go test -tags noEmbed -race`; build with `-tags noEmbed`; `golangci-lint run --build-tags noEmbed`.
- Fixture: `scripts/perf/gen-big-diff-fixture.sh <dest> full` → 407 files, 1,004,001 insertions, 39,104 hunks, 46.3 MB patch.

---

### Task 1: Outline scanner (O(hunks), streamed)

**Files:**
- Create: `api/internal/engine/git/internal/diff/outline.go`
- Create: `api/internal/engine/git/internal/diff/outline_test.go`
- Modify: `api/internal/engine/git/git.go` (Engine interface), `api/internal/engine/git/review_files.go` or a new `review_outline.go`

**Interfaces:**
- Consumes: `exec.GitStream` (Phase 1).
- Produces:
  ```go
  type HunkShape struct {
      OldStart int `json:"oldStart"`
      OldLines int `json:"oldLines"`
      NewStart int `json:"newStart"`
      NewLines int `json:"newLines"`
  }
  type FileOutline struct {
      Path      string      `json:"path"`
      OldPath   string      `json:"oldPath,omitempty"`
      Hunks     []HunkShape `json:"hunks"`
      IsPartial bool        `json:"isPartial"`
      IsBinary  bool        `json:"isBinary"`
  }
  Outline(ctx, repoPath, ref string) ([]FileOutline, error)
  ```
- Consumed by Task 4 (the HTTP handler) and Phase 3.

**Why:** `computeEstimatedDiffHeights` needs only hunk line counts, so the client can reserve correct scroll space per file before fetching any patch. Output is O(hunks) — 39,104 on the fixture, ~1.1 MB — never O(lines).

- [ ] **Step 1: Write the failing tests**

Cover, against small purpose-built repos: a single-hunk file; a multi-hunk file (assert exact `@@` ranges); a rename (`OldPath` set); a binary file (`IsBinary`, no hunks); a deleted file; an added file; a file whose hunk count exceeds `MaxOutlineHunksPerFile` (assert `IsPartial` and that hunks are capped); and an empty diff.

Add an equivalence test: for each file, the sum of `NewLines` across hunks must equal the `+` count `git diff --numstat` reports for that file, and `OldLines` the `-` count. That pins the parser against git itself rather than against your own expectations.

- [ ] **Step 2: Run to verify failure**

```bash
cd api && go test -tags noEmbed ./internal/engine/git/internal/diff/... -run Outline -v
```
Expected: `undefined: Outline`.

- [ ] **Step 3: Implement**

Stream `git diff -M <ref> --` through `exec.GitStream` with a `bufio.Scanner`, keeping only `diff --git`, `rename from/to`, `Binary files`, and `@@` lines. Parse `@@ -a,b +c,d @@` (both counts default to 1 when omitted — `@@ -5 +5 @@` is legal and means one line). Use `-U3`, matching the patch endpoint, so shapes match what the client renders.

**Scanner buffer:** `bufio.Scanner` defaults to a 64 KB max token and the fixture contains a 657k-character minified line. Either raise the buffer (`scanner.Buffer`) or use `bufio.Reader.ReadString`. A test with a >64 KB line is required — this is the failure mode that will otherwise appear only on real data.

`MaxOutlineHunksPerFile = 1000`: past it, stop collecting for that file, set `IsPartial`, and keep scanning to the next file.

- [ ] **Step 4: Verify + prove it streams**

```bash
cd api && go test -tags noEmbed -race -count=1 ./internal/engine/git/...
```

Then, in the scratchpad (no repo file), run `Outline` against the full fixture and report peak RSS and wall time. RSS must stay flat — comparable to Phase 1's 10.5 MB streaming figure, nowhere near the 200 MB buffering figure. Report both numbers and the hunk count (expect ~39,104).

---

### Task 2: Per-file patch reader

**Files:**
- Create: `api/internal/engine/git/internal/diff/patch.go`
- Create: `api/internal/engine/git/internal/diff/patch_test.go`
- Modify: `api/internal/engine/git/git.go`

**Interfaces:**
- Produces:
  ```go
  // FilePatch streams one file's unified patch into w. Returns the number of
  // patch lines written and whether it was truncated at maxLines.
  FilePatch(ctx, repoPath, ref, path string, maxLines int, w io.Writer) (lines int, truncated bool, err error)
  ```

**Why:** the backend already produces this text and currently spends CPU destroying it into per-line JSON (`diff.go:22`). Writing straight to an `io.Writer` means the daemon never holds the file's patch either.

- [ ] **Step 1: Write the failing tests**

Cover: a modified file (patch begins `diff --git`); a path that is not in the diff (zero lines, no error); a renamed file addressed by its NEW path; a binary file (git's `Binary files … differ` line, not content); truncation at `maxLines` cutting **at a hunk boundary** so the output is still a valid patch; `maxLines <= 0` meaning unlimited; and a pathspec-hostile filename (`:x`, `we*ird[1].txt`, a path with a space, a non-ASCII path) — reuse the `:(top,literal)` pathspec approach Phase 1 established in `file_summary.go`.

- [ ] **Step 2: Run to verify failure.** Expected: `undefined: FilePatch`.

- [ ] **Step 3: Implement.** Stream `git diff -M <ref> -- :(top,literal)<path>` and copy through, counting lines. On truncation, stop at the last complete hunk before the cap.

- [ ] **Step 4: Verify.** Full `-race` suite, plus a scratchpad run against the fixture's 420k-line monster file reporting RSS and wall time.

---

### Task 3: Streaming diff search

**Files:**
- Create: `api/internal/engine/git/internal/diff/search.go`
- Create: `api/internal/engine/git/internal/diff/search_test.go`
- Modify: `api/internal/engine/git/git.go`

**Interfaces:**
- Produces:
  ```go
  type SearchHit struct {
      Path       string `json:"path"`
      Side       string `json:"side"`       // "old" | "new"
      LineNumber int    `json:"lineNumber"`
      Preview    string `json:"preview"`
  }
  SearchDiff(ctx, repoPath, ref, query string, opts SearchOpts) (hits []SearchHit, truncated bool, err error)
  // SearchOpts: Regex, CaseSensitive bool; Limit int
  ```

**Why:** client-side find-in-diff is impossible once only a window is materialised. This is the approved replacement.

- [ ] **Step 1: Write the failing tests**

Cover: a literal match on an added line (`Side: "new"`, correct `LineNumber`); a match on a removed line (`Side: "old"`); a match in **context** (must be attributed to the new side with the correct new-side number); line numbers tracked correctly across multiple hunks and multiple files (the core risk — assert exact numbers against a hand-built repo); `Limit` reached → `truncated` true and exactly `Limit` hits; regex mode; case-insensitive mode; an invalid regex → error, not panic; no matches → empty, not nil-error; a match inside the minified >64 KB line (preview must be truncated, not the whole line); and a query matching a `@@` header or `diff --git` line, which must NOT be reported as a content hit.

- [ ] **Step 2: Run to verify failure.**

- [ ] **Step 3: Implement.** Stream the diff, tracking current file from `diff --git`, and old/new line counters from each `@@` header — incrementing per line by prefix (`+` advances new, `-` advances old, ` ` advances both). Cap `Preview` (say 200 chars).

- [ ] **Step 4: Verify.** Full `-race` suite plus a fixture run: search for a token present in every scattered file, report hit count, wall time and RSS.

---

### Task 4: HTTP endpoints

**Files:**
- Create: `api/internal/api/v0/endpoints/review/handlers/outline.go`, `patch.go`, `search.go` (+ tests)
- Modify: `api/internal/api/v0/endpoints/review/routes.go`, `api/internal/api/v0/route_audit_test.go`
- Modify: `api/internal/app/usecases/branchreview/` — a usecase method per endpoint, resolving the diff ref exactly as `GetFiles` does

**Interfaces:**
- `GET /v0/projects/:projectId/repos/:repoId/workspaces/:wsId/review/outline` → `{files: FileOutline[]}`
- `GET …/review/patch?path=<p>&maxLines=<n>` → `text/plain`, header `X-Crowbar-Diff-Truncated: true` when cut
- `GET …/review/search?q=&regex=&case=&limit=` → `{hits: SearchHit[], truncated: bool}`

**Requirements:**

- Reuse `resolveDiffRef` — do not re-derive the base ref; a different ref here than in `/review/files` would show the user two inconsistent diffs.
- `path` is required on `/patch`; missing or empty → 400.
- `limit` defaults to 200, caps at 1000.
- The patch endpoint must stream to `ctx.Writer`, not build a string. **A test must prove this**: assert the handler writes incrementally (e.g. via a flushing recorder), because a buffering implementation is otherwise indistinguishable.
- Cache the outline under the same `(ref, headSHA)` key family Phase 1's `summary_cache.go` uses. Read that file and follow its eviction and key discipline rather than inventing a second scheme.
- Declare the three routes in `route_audit_test.go`'s `extraRoutes()`. Note: `TestRouteAudit_AllSpecRoutesRegistered` is **red at baseline** for two unrelated pre-existing routes (recorded in Phase 0) — make sure your three are not among the failures, and say so.
- Add a `perf_wiring`-style test proving all three mount on the **real** router, as Phase 0's `perf_wiring_test.go` does for `/system/perf`.

- [ ] **Step 1-4:** failing tests → verify red → implement → full `-race` suite + `golangci-lint` + real-router mount proof.

- [ ] **Step 5: End-to-end against the live daemon**

Rebuild the sidecar (`bash desktop/scripts/fetch-sidecar.sh`, copy over `desktop/src-tauri/target/debug/crowbar-api`, restart on the existing socket — the app reconnects). Then, against the `PerfFixture` / `big-diff` workspace already registered in the dev home, `curl` all three endpoints and report: outline payload size and hunk count, patch size and time for the 420k-line monster, and search hit count and time. Arm `/v0/system/perf` first and include the daemon sample breakdown. Disarm afterwards.

---

## Definition of done

- [ ] `go test -tags noEmbed -race ./...` passes; `golangci-lint` no new findings.
- [ ] Outline on the fixture: ~39,104 hunks, RSS flat, payload ~1.1 MB.
- [ ] Patch for the 420k-line monster streams without a diff-sized RSS spike.
- [ ] Search over the 46 MB patch returns correct file+line hits, RSS flat.
- [ ] All three endpoints verified on the **real** router and against the **live** daemon.
- [ ] `/review` and `/review/files` behaviour unchanged; no frontend file touched.
