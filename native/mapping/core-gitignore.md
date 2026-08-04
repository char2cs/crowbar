# `crowbar-core::filetree::gitignore` — the `.gitignore` cascade (P3.76)

One file from `native/mapping/tier-a-denominator.md` §5's file-tree-model
area — `file-explorer/lib/file-tree-gitignore.ts` (the real, nested path is
`features/file-explorer/file-explorer/lib/file-tree-gitignore.ts`;
`features/file-explorer/lib/file-tree-gitignore.ts` is a 1-line
`export * from '../file-explorer/...'` re-export shim, confirmed by reading
both files directly rather than trusting the directory listing) — ported into
`native/crates/crowbar-core/src/filetree/gitignore.rs`. All 7 exports, all
confirmed LIVE at export level (§5's per-export table, `tier-a-denominator.md`
line 1982: *"all 7 exports LIVE... Zero dead exports"*). This is the first
module in a new `crowbar-core::filetree` area; a sibling item
(`native/p3.75-core-filetree`) ports the rest of §5 concurrently in its own
worktree, so `lib.rs` and `filetree/mod.rs` were kept minimal and additive.

## 1. What was ported, and where each TS export landed

| TS export | Rust item |
|---|---|
| `isWorkspaceRelativePath` | [`is_workspace_relative_path`] |
| `GitIgnoreFileReference` | [`GitIgnoreFileReference`] |
| `GitIgnoreFileContent` | [`GitIgnoreFileContent`] |
| `FileTreeGitIgnoreRules` | [`FileTreeGitIgnoreRules`] |
| `collectGitIgnoreFileReferences` | [`collect_git_ignore_file_references`] |
| `createFileTreeGitIgnoreRules` | [`create_file_tree_git_ignore_rules`] |
| `isPathGitIgnoredByFileTreeRules` | [`is_path_git_ignored_by_file_tree_rules`] |

Everything else in the file (`GITIGNORE_FILE_NAME`, `isRootDirectoryPath`,
`isPathInRootScope`, `pathDepth`, `compareIgnoreReferences`,
`addGitIgnoreContent`, `toMatcherPath`, `isPathIgnoredByOwnRules`, and the
**local** `getAncestorDirectoryPaths`) are private helpers, ported as
non-`pub` functions in the same module.

**The path-helper subset.** The TS file imports `getDirName`/
`getRelativePath`/`normalizePath`/`pathStartsWithRoot`/
`stripTrailingPathSeparators` from `web/src/utils/path-helpers.ts` — a
separate file, out of this item's scope (`file-tree-gitignore.ts`'s own 7
exports only, per the brief). Rather than either skip them (which would not
compile) or silently port the whole of `path-helpers.ts` under this item
(scope creep the brief explicitly warned against), the five functions this
file actually calls were re-implemented as **private, non-exported**
functions local to `gitignore.rs`, narrowed to the exact signatures this
file's call sites use (e.g. `getRelativePath`'s TS signature accepts
`rootFolderPath: string | null | undefined`; every call site here always
passes a concrete non-empty `&str`, so the port narrows to that). If a future
item ports `crowbar-core::path_helpers` as its own module, it is free to
define the full shape without colliding with a name this file already used.

**The tree-entry type.** `collectGitIgnoreFileReferences`'s `files: FileEntry[]`
parameter references `web/src/features/file-system/types/app.ts`'s
`FileEntry` (= `AppFile`), also out of this item's scope. Ported as
[`GitIgnoreTreeEntry`] — a narrow struct carrying only the four fields this
function reads (`name`, `path`, `is_dir`, `children`), deliberately **not**
named `FileEntry` so it does not collide with whatever name a fuller
`AppFile`/`FileEntry` port (§5's own scope, not this item's) picks later.

## 2. The dependency decision — verified, not assumed

**Recommendation, as given:** use the Rust `ignore` crate
(`ignore::gitignore::GitignoreBuilder`/`Gitignore`) instead of hand-rolling a
`.gitignore` matcher, one `Gitignore` instance per directory mirroring the TS
file's own `ruleSets` shape.

**Verified before building on it, per the brief's explicit instruction not to
assume the recommendation:**

- **Availability.** `ignore` is not a new dependency in the practical sense:
  `cargo tree -i ignore` (run before touching any manifest) shows it already
  in this workspace's `Cargo.lock` transitively — `gpui-component -> rust-i18n
  -> globwalk -> ignore 0.4.31`. Declaring it as a direct `crowbar-core`
  dependency resolves to the same cached 0.4.31 with no new network fetch,
  confirmed by `cargo check -p crowbar-core` completing without any registry
  download.
- **`Match` semantics.** Read directly from the vendored crate source
  (`~/.cargo/registry/.../ignore-0.4.31/src/{lib,gitignore}.rs`), not taken on
  the brief's word. `ignore::Match<T>` is `{None, Ignore(T), Whitelist(T)}`;
  `Gitignore::matched()`'s own doc comment says it returns "the highest
  precedent glob," i.e. the *last* matching glob in file order wins — the
  crate's own tests `ig7` (`"!src/main.rs\n*.rs"` on `"src/main.rs"` →
  ignored, because the un-negated pattern comes after the negation) and
  `ignot6` (same two lines reversed → not ignored) are the identical
  precedence rule the npm `ignore` package documents for its `unignored`
  field. So the mapping is a direct structural match: `{ignored:true,
  unignored:false}` ↔ `Match::Ignore`, `{ignored:false, unignored:true}` ↔
  `Match::Whitelist`, `{ignored:false, unignored:false}` ↔ `Match::None`.
  `is_path_ignored_by_own_rules` folds a `Match` into the same
  `ignored`-accumulator loop the TS source uses for `result.ignored`/
  `result.unignored` — a direct translation, not a reimplementation.
- **One real divergence, found and resolved, not papered over.** TS's
  `toMatcherPath` signals "this candidate is a directory" by *appending a
  trailing `/`* to the string handed to `matcher.test()`, because npm
  `ignore`'s `test(path)` takes exactly one argument. `Gitignore::matched(path,
  is_dir)` instead takes an explicit `is_dir: bool` as a *second* parameter
  and does not expect a trailing separator — the crate's own test suite
  matches bare paths (`"foo"`, not `"foo/"`) against directory-only patterns,
  signalling directory-ness only through the boolean (`ig8`/`ig27`/`ig30`).
  Appending a trailing slash *and* passing `is_dir: true` would double-signal
  in a way the crate's compiled-glob matcher does not expect (a pattern's own
  trailing `/` is stripped before compilation — see `GitignoreBuilder::add_line`
  — so a candidate that still has one risks a spurious segment mismatch).
  [`to_matcher_path`] therefore does **not** port the trailing-slash append;
  directory-ness is carried solely through `is_dir`. Verified, not just
  reasoned about: `tests::handles_anchored_nested_patterns_and_directory_only_patterns`
  (ported directly from the TS suite's identically-named case, exercising a
  `build/`-style directory-only pattern) passes with this approach.
- **What the crate does not provide.** The **ancestor-first cascade**
  (§3 below) — `ignore::WalkBuilder`/`Ignore` do something related during
  filesystem *traversal*, but this file never walks a filesystem; it resolves
  rules against an already-loaded, in-memory tree. No crate type does that.

**No divergence found in ordering of negations, directory-only patterns, or
anchoring** beyond the trailing-slash mechanism above — the four mutations in
§4 below exercise negation ordering and directory-only patterns directly and
all behaved as the TS source's own tests predict.

## 3. The file's real contribution: the ancestor-first cascade

`isPathGitIgnoredByFileTreeRules` walks every ancestor directory (via the
**local, live** `getAncestorDirectoryPaths` at TS source line 206) and tests
each ancestor's own accumulated rules *before* testing the target path,
because a directory ignored by a parent rule ignores everything beneath it
regardless of that subdirectory's own `.gitignore`. This has no crate
equivalent and is reimplemented exactly as the TS source structures it.

**The dead-twin note.** `getAncestorDirectoryPaths` exists twice in the
codebase: an exported copy in `file-explorer-tree-utils.ts` that
`tier-a-denominator.md` §5's export-level table confirms is **TEST-ONLY**
(zero non-test, zero self-file references — its only exerciser is that
file's own dedicated test), and the local, unexported copy at
`file-tree-gitignore.ts:206` that actually does the ancestor-walk work for
every production gitignore resolution. This port ships the local, live copy
only ([`get_ancestor_directory_paths`], private to this module) and does not
resurrect or duplicate the dead exported twin — matching the brief's explicit
instruction.

## 4. Mutation testing — 4 mutations, ≥2 on the ancestor cascade

All four were applied by editing the source, run to a real failure, the
failure output captured, then reverted and re-verified green (with a `touch`
after every revert to defeat the mtime trap where restoring old content can
leave `cargo test` serving a stale binary that looks like the test never
failed).

1. **Ancestor cascade — `get_ancestor_directory_paths` returns `vec![]`
   unconditionally.** 5 tests failed, including both new
   `cascade_*` tests:
   ```
   ---- ...::cascade_ignores_a_file_two_levels_beneath_an_ignored_ancestor stdout ----
   thread '...' panicked at .../gitignore.rs:1098:9:
   assertion failed: is_path_git_ignored_by_file_tree_rules(rules.as_ref(),
       "/repo/build/nested/deep/output.js", false)

   ---- ...::keeps_files_ignored_when_an_ancestor_directory_is_ignored stdout ----
   thread '...' panicked at .../gitignore.rs:769:9:
   assertion failed: is_path_git_ignored_by_file_tree_rules(rules.as_ref(), "/repo/logs/keep.log",
       false)
   test result: FAILED. 19 passed; 5 failed;
   ```
   Reverted; 24/24 green again (this was before the coverage-closing tests
   were added, hence 24 not 28).

2. **Ancestor cascade — test each ancestor directory as `is_dir: false`
   instead of `true`.** 3 tests failed (exactly the ones whose ancestor rule
   is a directory-only pattern — `logs/`, `dist/`, `build/`):
   ```
   ---- ...::keeps_files_ignored_when_an_ancestor_directory_is_ignored stdout ----
   thread '...' panicked at .../gitignore.rs:767:9:
   assertion failed: is_path_git_ignored_by_file_tree_rules(rules.as_ref(), "/repo/logs/keep.log",
       false)
   test result: FAILED. 21 passed; 3 failed;
   ```
   Reverted; green again.

3. **Negation precedence — swap the `Match::Ignore`/`Match::Whitelist`
   accumulator arms** (`ignored = true`/`ignored = false` reversed). 14 of 24
   tests failed — this breaks the core `{ignored, unignored}` semantics the
   whole dependency-equivalence argument in §2 rests on:
   ```
   test result: FAILED. 10 passed; 14 failed;
   failures:
       ...::applies_nested_gitignore_files_relative_to_the_directory_that_owns_them
       ...::does_not_treat_the_repository_root_or_git_directory_as_ignored
       ...::handles_anchored_nested_patterns_and_directory_only_patterns
       ...::keeps_files_ignored_when_an_ancestor_directory_is_ignored
       ...::lets_lower_gitignore_files_unignore_files_ignored_by_parent_rules
       ...::supports_windows_paths_after_normalizing_matcher_input
       (+ 8 more)
   ```
   Reverted; green again.

4. **The `.git` special-case guard** — mutated the two string comparisons
   (`root_relative == ".git"` / `".git/"`) to strings that can never match.
   Exactly the 2 tests that exist to prove this guard failed, nothing else:
   ```
   ---- ...::workspace_relative_paths_synthetic_root::does_not_treat_the_relative_git_directory_as_ignored stdout ----
   thread '...' panicked at .../gitignore.rs:991:13:
   assertion failed: !is_path_git_ignored_by_file_tree_rules(rules.as_ref(), ".git", true)

   ---- ...::does_not_treat_the_repository_root_or_git_directory_as_ignored stdout ----
   thread '...' panicked at .../gitignore.rs:1030:9:
   assertion failed: !is_path_git_ignored_by_file_tree_rules(rules.as_ref(), "/repo/.git", true)
   test result: FAILED. 22 passed; 2 failed;
   ```
   Reverted; green again, `cargo fmt --check` and `cargo clippy` re-confirmed
   clean after the round trip.

## 5. Tests — ported vs. authored

28 tests total in `filetree::gitignore::tests`.

- **18 ported directly from
  `web/src/__tests__/features/file-explorer/file-tree-gitignore.test.ts`**
  (17 `it`/`it.each` call sites; the `it.each(['', '.'])` case expands to 2
  Rust `#[test]` functions, matching its 2 parameterized runs) — every case
  in the TS suite has a same-behavior Rust counterpart, same fixture shapes,
  same assertions.
- **10 authored for this port**, closing gaps the TS suite didn't cover:
  `is_workspace_relative_path`'s three-way branch (leading `/`, leading `\`,
  drive letter) in isolation; `None`/empty-root early returns on all three
  public entry points; `create_file_tree_git_ignore_rules` returning `None`
  when every candidate is filtered out of root scope (as opposed to an empty
  input list, a different branch); the workspace-relative-ruleset-under-an-
  absolute-root mismatch case (§2's `to_matcher_path` early return); and the
  two `cascade_*` tests written specifically for the ancestor-walk (§3),
  since the TS suite's own ancestor-cascade coverage is a single case
  (`keeps_files_ignored_when_an_ancestor_directory_is_ignored`, ported as
  test #6 above) and the brief asked for the cascade to be "the part worth
  testing hardest."

## 6. Coverage

`cargo llvm-cov -p crowbar-core`:

| | lines | line coverage |
|---|---|---|
| crate total, before this item | 2,531 | 100.00% |
| `filetree/gitignore.rs` alone | 636 | 98.27% (11 lines missed) |
| crate total, after this item | 3,167 | 99.65% (11 lines missed) |

The "before" figure was measured directly (not computed from a stale
baseline): `git stash push -u` on every P3.76 change, re-ran
`cargo llvm-cov -p crowbar-core --summary-only` against the untouched tree
(2,531 lines, 100.00%, 231 tests), then `git stash pop`.

The 11 uncovered lines in `gitignore.rs` are concentrated in the private
path-helper subset (§1) and in `is_path_ignored_by_own_rules`'s own
`rules: None` / out-of-root-scope early returns: defensive branches that
mirror redundant checks the TS source itself carries (e.g.
`isPathIgnoredByOwnRules`'s own `!rules` guard, dead by construction once its
one caller has already checked `Option::is_some()`), unreachable through this
module's public entry points by the same call-graph argument that makes them
effectively dead in the TS source too. This 98.27%/99.65% is consistent with
—not an outlier against — the rest of the crate: `settings/normalization.rs`
sits at 97.96%, `keymap/chord.rs` at 96.62%, `workspace/scope.rs` at 99.72%,
none of them 100%, for the same class of reason.

## 7. What this item found, checked against the brief's premises

- **The dependency recommendation held up under verification** — see §2.
  The one real API-shape divergence found (trailing-slash-vs-`is_dir`-bool)
  was resolvable without weakening the port; it does not indicate the crate
  choice was wrong, only that the two APIs signal directory-ness differently.
- **The "test-only twin" claim (`getAncestorDirectoryPaths` in
  `file-explorer-tree-utils.ts`) checked out exactly as stated** — confirmed
  independently by reading `tier-a-denominator.md`'s export-level table
  rather than taking the brief's summary at face value; the local copy at
  line 206 is what's ported, per §3.
- **The two source-file paths (`features/file-explorer/lib/...` vs.
  `features/file-explorer/file-explorer/lib/...`) checked out as a genuine
  1-line-shim-vs-237-line-real-file split**, confirmed with `wc -l` on both
  before reading either, matching the brief's own description exactly.
- **Nothing in this pass contradicted the brief.** The scope (7 exports),
  the dependency choice, the "port the local copy, not the exported one" note,
  and the crate-availability claim (already in `Cargo.lock` transitively) all
  checked out against direct evidence rather than being taken on trust.
