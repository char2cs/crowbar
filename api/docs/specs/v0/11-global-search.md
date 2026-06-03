# Crowbar Backend — Global Search

> **Status:** Approved
> **Date:** 2026-06-03
> **Depends on:** `00-architecture-and-domain.md`, `02-api-surface.md`,
> `05-filesystem-and-watcher.md`
> **Scope:** Full-text search and replace across a workspace's repo. Covers UX
> spec §25.

---

## 1. Decision — Own Pure-Go Engine (no external dependency)

Search is implemented **in-process in pure Go**, not by shelling out to `rg`.

Rationale: unlike `git`, `gh`, and the language servers — which are *intrinsic*
dependencies (you cannot do git without git) — search can run entirely
in-process. There is **no well-maintained embeddable Go library** that is a
drop-in ripgrep (sift/pt are abandoned CLIs; `google/codesearch` is index-based
and wrong for a live-editing workspace; the `rg` wrapper packages just shell out).
So the realistic choices were "shell out to `rg`" or "compose our own from
stdlib + two small libs." We choose our own: **zero extra mandatory install,
self-contained in the binary, identical across platforms.**

The heavy lifting (regex engine, fast directory walk) is already in Go's stdlib;
what we write is glue. Estimated ~200–300 lines + tests. The trade-off is being
slower than `rg` on very large repos — irrelevant for single-workspace search.

---

## 2. Package Structure

```
internal/engine/search/
  search.go           SearchEngine interface
  internal/
    walk/             concurrent directory walk (filepath.WalkDir / fastwalk)
    ignore/           .gitignore hierarchy + hardcoded .git skip
    match/            regexp build from toggles; byte-offset extraction; binary skip
    replace/          apply replacements (single file / across files)
```

---

## 3. Option → Implementation Mapping

Every UX toggle maps onto Go's `regexp` (RE2) plus two small libs:

| UX option | Implementation |
|-----------|----------------|
| Case-sensitive | `regexp` with/without the `(?i)` flag |
| Whole word | wrap the pattern in `\b…\b` |
| Regex mode | use the pattern directly |
| Fixed-string (regex off) | `regexp.QuoteMeta` the query first |
| Include glob | `bmatcuk/doublestar` path match (keep) |
| Exclude glob | `bmatcuk/doublestar` path match (drop) |
| Respect `.gitignore` | `ignore/` package (hierarchical, see §5) |
| Skip `.git/` | hardcoded |
| `matchStart` / `matchEnd` | `regexp.FindAllIndex` byte offsets (§6) |

Binary files are skipped via a null-byte sniff (same check as the fs content
reader, `05-filesystem-and-watcher.md`).

---

## 4. Concurrency

`walk/` feeds file paths into a **bounded worker pool**; each worker reads a
file, runs the compiled regexp, and emits `SearchResult`s. The pool size is
bounded (e.g. `GOMAXPROCS`) so a large repo does not exhaust file descriptors.
Results are collected and returned per request (the frontend debounces input and
groups by file, UX §25).

---

## 5. `.gitignore` Hierarchy — the fiddly part

`.gitignore` is **hierarchical**: every directory may carry its own
`.gitignore`, patterns can be negated (`!`), and deeper rules override shallower
ones. The `ignore/` package:

- Loads the ignore stack as the walk descends (root → subdirectory).
- Uses a gitignore-matching lib (`denormal/go-gitignore` or equivalent) for the
  pattern semantics, while **we** assemble the per-directory stack and precedence.
- Always skips `.git/` regardless of ignore files.

This is the one genuinely fiddly area; it shares intent (but not code) with the
file-watcher's ignore rules (`05-filesystem-and-watcher.md` §2), which also honor
`.gitignore`. Keeping a single `ignore/` implementation reusable by both is
preferred.

---

## 6. Search Endpoint

```
POST /v0/workspaces/:wsId/search
{ query, caseSensitive, wholeWord, regex, include[], exclude[] }
→ SearchResult[]

SearchResult { filePath, lineNumber, lineText, matchStart, matchEnd }
```

`matchStart` / `matchEnd` are byte offsets from `FindAllIndex`. If Monaco needs
character (UTF-16) columns, `match/` converts before returning — a small,
contained mapping step.

**Result cap.** Results are capped (default **1000** matches, configurable) so a
broad pattern (e.g. `.` on a large repo) cannot return millions of rows. The
response includes a `truncated: bool` so the UI can show "showing first N"
(consistent with the no-silent-caps principle — the cap is reported, not hidden).

---

## 7. Replace Endpoint

```
POST /v0/workspaces/:wsId/search/replace
{ query, replacement, scope }      // scope: one file or all matches
```

`replace/` rewrites file content on disk (supporting regex backreferences in
`replacement`). Each write triggers the **file watcher**, which fans out to the
Files / Git / Workspaces topics (`05-filesystem-and-watcher.md` §5) — so the
editor, tree, and git panel update live, with no special-casing for replace.

---

## 8. Out of Scope

- Shelling out to `rg` (rejected — §1).
- Trigram / persistent index (wrong fit for live-editing; rejected — §1).
- Cross-workspace / whole-disk search (search scope is the workspace repo).
