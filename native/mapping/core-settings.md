# `crowbar-core::settings` — the settings-schema area (P3.72)

Eight files from `native/mapping/tier-a-denominator.md` §4 ("Settings
schema") ported into `native/crates/crowbar-core/src/settings/`, the fourth
Tier A area to land after workspace scoping (P3.53), the git model (P3.67)
and keymap resolution (P3.70). Scope was the settings-schema **LIVE**
subset only — reachable at boot, before any dialog exists to open
(`main.tsx` → `initializeSettingsStore` → `settings-bootstrap.ts` calls
`default-settings.ts` and `normalizeSettings` unconditionally, per §4's own
Liveness section). Each module carries its own doc comment citing the TS
source and any surprising behaviour; this file is the cross-module index:
what each module models, the reconciliation this item's brief asked for
before writing any Rust, what was deliberately not ported and why, the
mutation-testing summary, and the coverage/gate results.

## 0. Reconciliation against the survey, done before writing any Rust

The brief listed eight files and asked for them to be checked against §4's
own "unique 9-file reconstruction totalling 554 LIVE + 75 CONDITIONAL"
before starting. The reconciliation section at the end of
`tier-a-denominator.md` (`### Denominator reconciliation (P3.71)`, table
"(B) Tier A core only") spells the 9-file, 629-line settings-schema Tier A
core figure out explicitly:

- **554 LIVE**: `types/settings.ts` (81), `types/feature.ts` (3),
  `default-settings.ts` (98), `typography-defaults.ts` (25),
  `settings-normalization.ts` (249), `font-family-resolution.ts` (40),
  `markdown-font-size.ts` (26), `ui-font-size.ts` (32) — **exactly the eight
  files this item was scoped to, summing to exactly 554.**
- **75 CONDITIONAL**: `settings-import-export.ts` — gated behind the
  Settings → Developer tab's export/import buttons (§4's own per-file
  Liveness table confirms this verdict independently).

**Result: no ninth LIVE file was missed, and nothing in the eight needed
dropping.** The brief's eight files are exactly the LIVE half of the
survey's own 9-file reconstruction; the ninth file
(`settings-import-export.ts`) is correctly CONDITIONAL and therefore
correctly out of this item's scope. This was checked against the survey's
per-file Liveness table (§4, the row-by-row breakdown) independently of the
reconciliation table, and both agree.

## 1. What each module models

| module | ported from | TS tests? |
|---|---|---|
| [`types`] | `types/settings.ts` + `types/feature.ts` | none — authored here |
| [`typography`] | `config/typography-defaults.ts` | none (pure data, no test file in TS either) |
| [`defaults`] | `config/default-settings.ts` | none — authored here (see §3) |
| [`normalization`] | `lib/settings-normalization.ts` | `settings-normalization.test.ts` (15 cases) + `settings-normalization-theme.test.ts` (4 cases), ported, plus 7 authored |
| [`font_family`] | `lib/font-family-resolution.ts` | `font-family-resolution.test.ts` (6 cases), ported, plus 2 authored |
| [`markdown_font_size`] | `lib/markdown-font-size.ts` | `markdown-font-size.test.ts` (6 cases), ported, plus 2 authored |
| [`ui_font_size`] | `lib/ui-font-size.ts` | `ui-font-size.test.ts` (5 cases), ported, plus 2 authored |
| [`raw_value`] | (new — see below) | n/a |

- **`types`** — the schema itself: [`types::Settings`] (~50 fields:
  general/editor/terminal/UI/theme/layout/language/file-tree) and
  [`types::CoreFeaturesState`]. Six closed-domain fields
  (`sidebar_position`, `editor_engine`, `render_whitespace`,
  `terminal_cursor_style`, `theme_mode`, `external_editor`) are Rust enums
  rather than `String`, matching `crate::keymap::types`'s precedent
  (`CommandCategory`, `KeymapPresetId`) for the same reason: a TS string
  union is a compile-time-only promise, and sealing it makes the invalid
  states `settings-normalization.ts` exists to catch unrepresentable by
  construction. `theme`/`icon_theme`/`auto_theme_light`/`auto_theme_dark`
  stay `String` — TS types `Theme` as a bare `string` because the valid set
  is a *runtime* registry (built-ins plus session-uploaded custom themes),
  not a closed union, so there is nothing to seal.
- **`typography`** — font/size constants, ported verbatim (pure data in the
  TS source too).
- **`defaults`** — [`defaults::default_settings`] (the `defaultSettings`
  object) and [`defaults::DEFAULT_THEME_ID`]. See §3 for why
  `getDefaultSetting`/`getDefaultSettingsSnapshot` have no direct
  counterpart.
- **`normalization`** — validation, clamping and migration across ~15
  fields, the substance of this area. [`normalization::normalize_settings`]
  is the whole-object entry point `settings-bootstrap.ts` calls at boot;
  the module also exposes standalone `parse_*`/`normalize_*` functions per
  clamped/migrated field (clamp: editor line height, file-tree indent size,
  workspace-keep-alive minutes, ui/markdown font size; migrate: terminal
  line height's legacy default, theme-id registry coercion, the two
  `themeMode` behaviours; fallback: render whitespace, editor engine,
  external editor). See §4 for the four asymmetries this enumeration found
  in the original TS source, and §5 for why several of the ported TS test
  cases moved from `normalize_settings` to these standalone functions.
- **`font_family`** — font-family parse/normalize/resolve-against-available,
  all three exports (`get_primary_font_family`,
  `normalize_configured_font_family`, `resolve_available_font_family`).
- **`markdown_font_size`** / **`ui_font_size`** — clamp/snap (and, for UI
  font size, scale) one numeric field each. Both accept a
  [`raw_value::RawNumber`] rather than a bare `f64`, so the TS source's
  "accepts a numeric string too" behaviour (`normalizeMarkdownFontSize('20')
  === 20`) has a faithful Rust representation.
- **`raw_value`** — new, not a port of anything. [`raw_value::RawNumber`] is
  a minimal stand-in for TS's `unknown`, scoped to exactly the two shapes
  `ui_font_size`/`markdown_font_size`/`normalization`'s
  `normalize_workspace_keep_alive_minutes` branch on (a genuine number, or a
  string that might parse as one). See its module doc for why two variants
  are enough and what the third case ("anything else": `null`, `undefined`,
  an object) folds into.

## 2. A cross-area dependency this item could not defer

`settings-normalization.ts` imports `normalizeFileTreeDensity` from
`web/src/features/file-explorer/lib/file-tree-density.ts` — File-tree model
(§5 of the survey), a *different* Tier A area not yet ported to
`crowbar-core`. That import is unconditional in the boot-time
`normalizeSettings` path, so this item could not simply omit it without
leaving `normalize_settings` unable to reproduce real boot behaviour.
[`types::FileTreeDensity`] and [`types::normalize_file_tree_density`] are a
narrow, explicitly-flagged local duplicate of that file's 3-variant type +
normalizer (see `types`'s own module doc for the full reasoning) —
**when file-tree model is ported, this type should be deleted in favour of
that module's, and every reference here repointed.** Flagged here rather
than silently left as a permanent fork.

## 3. Two TS exports with no direct Rust counterpart

- **`getDefaultSetting<K>(key: K): Settings[K]`** — a generic keyed
  accessor. Rust has no ergonomic equivalent of TS's `K extends keyof
  Settings` dispatch without a hand-rolled enum-of-field-names that would
  just re-encode a plain field access with no behaviour of its own.
  `default_settings().foo` is the idiomatic spelling.
- **`getDefaultSettingsSnapshot(): Settings`** — exists to defend against a
  JS-specific hazard this port does not have: `{...defaultSettings}` is a
  *shallow* copy, so two "snapshots" would share the same `coreFeatures`
  object and pattern arrays by reference. Every `Settings` field in this
  port is owned (`String`, `Vec<String>`, a `Copy` struct); returning
  `default_settings()` by value is already fully independent — the same
  "the type system already rules out the bug" shape
  `crate::git::normalize_diff`'s module doc documents for a different
  field. The function's other job — re-running `uiFontSize`/
  `markdownFontSize` through their own clamps defensively — ports as an
  invariant test instead (`default_font_sizes_are_already_fixed_points_of_their_own_clamp`,
  in `defaults.rs`), proving the same fact the redundant TS call was
  establishing.

## 4. Four asymmetries in the original source, found while enumerating every branch

Per this item's own verification requirement — "enumerate the real branches
… what a missing field falls back to, what an out-of-range persisted value
migrates to" — reading `settings-normalization.ts` line by line surfaced
four real, live asymmetries, preserved rather than "fixed" (each has a
dedicated test):

1. **`normalizeFileTreeIndentSize`'s non-finite fallback is `20`, not the
   schema default of `16`.** `defaultSettings.fileTreeIndentSize` is `16`;
   the clamp function's own `!Number.isFinite(value)` branch returns a
   literal `20`. Two independently-declared numbers with no shared constant
   between them in the TS source — most likely a bug (a NaN-shaped
   persisted value migrates to a value the user never chose and the schema
   doesn't call "default"), but it is the actual, live behaviour.
   Reproduced exactly, with a dedicated test
   (`file_tree_indent_size_non_finite_fallback_is_20_not_the_schema_default_of_16`)
   and one of this item's five mutations (§6).
2. **`normalizeEditorEngine`'s `customEditorCommand` parameter is unused.**
   The TS signature is `normalizeEditorEngine(value: unknown,
   _customEditorCommand: string | undefined)` — underscore-prefixed, dead.
   The real "a `custom` engine with no command string falls back to
   `monaco`" rule lives entirely in `normalizeSettings`'s own body, applied
   *after* this function returns. [`normalization::normalize_editor_engine`]
   takes no such parameter at all; the cross-field rule lives in
   [`normalization::normalize_settings`], matching where the TS source
   actually puts it.
3. **`externalEditor` has no branch in `normalizeSettingValue` at all.**
   Every other enum-ish field has a `key === '…'` branch there;
   `externalEditor` does not, so a direct per-key update of that setting is
   never validated in the TS source — only a full `normalizeSettings` pass
   (e.g. at next boot) would catch a bad value. Not closed here either:
   inventing a per-key entry point this port doesn't otherwise have, to
   validate a field the TS source itself never validates that way, would be
   adding behaviour instead of porting it.
4. **`themeMode` has two unrelated normalizers, not one.**
   `normalizeSettings`'s inline block only checks *falsiness*
   (`!normalizedSettings.themeMode`) and, when falsy, derives from
   `syncSystemTheme` — it does **not** validate an already-truthy value, so
   `normalizeSettings({..., themeMode: 'invalid'})` leaves `'invalid'`
   untouched in the TS source. `normalizeSettingValue('themeMode', …)`, by
   contrast, validates membership and falls back to `'system'` — but never
   consults `syncSystemTheme`. Ported as two distinctly-named functions,
   [`normalization::migrate_theme_mode_from_sync_system_theme`] and
   [`normalization::normalize_theme_mode_value`], rather than one merged
   function that would silently change which cases each behaviour covers.

## 5. Why `normalize_settings` doesn't take every TS test case verbatim

[`types::Settings`] seals six fields as Rust enums (§1). By construction, a
`Settings` value can never actually hold the malformed shapes several TS
test cases construct on purpose — `editorEngine: 'emacs' as never`,
`themeMode: undefined as unknown as ThemeMode`. This is the exact situation
`crate::git::normalize_diff`'s module doc already worked through for a
different field: *"the field can't lie about its own shape … there is no
runtime state a normalize function could observe and repair that the type
doesn't already rule out."* Two ported-TS-test-case shapes are genuinely
unreachable once a `Settings` is already constructed:

- **`normalizeSettings({..., editorEngine: 'emacs' as never})` →
  `'monaco'`.** Relocated to
  `normalize_editor_engine_falls_back_to_monaco_for_a_value_the_schema_no_longer_has`,
  which calls [`normalization::normalize_editor_engine`] directly with the
  raw string `"emacs"` — the boundary where an untrusted string can still
  exist.
- **`normalizeSettings({..., themeMode: undefined as unknown as
  ThemeMode, syncSystemTheme: true/false})` → `'system'`/`'light'`.**
  Relocated to
  `sets_theme_mode_to_system_when_sync_system_theme_was_true_and_the_value_is_missing`
  / `..._false_...`, which call
  [`normalization::migrate_theme_mode_from_sync_system_theme`] directly
  with `None`.

Every other clamp/migration/cross-field rule (font family, terminal line
height, editor line height, file-tree indent size, workspace-keep-alive
minutes, the custom-engine/external-editor precedence, theme-id coercion)
*is* still meaningful on an already-typed `Settings` — an `f64` field can
still hold `NaN`/`Infinity`/an out-of-range value, and `theme: String` has
no seal at all by design — and is ported as a method on the whole struct,
exercised through `normalize_settings` exactly as the TS source does.

## 6. Mutation testing

Five mutations run (the brief asked for at least four, including one on a
clamp boundary and one on a migration path); each was watched to fail for
real, then reverted. Full transcripts are pasted into this section rather
than summarized, per this item's own evidentiary requirement.

1. **Clamp boundary — `FILE_TREE_INDENT_SIZE_MAX` 32.0 → 40.0.**
   `file_tree_indent_size_clamps_below_at_and_above_the_supported_range`
   failed:
   ```
   thread '...' panicked at crates/crowbar-core/src/settings/normalization.rs:417:9:
   got 40, want 32
   test result: FAILED. 1 passed; 1 failed
   ```
   Reverted.
2. **Migration path — `is_legacy_terminal_line_height` forced to always
   return `false`.**
   `migrates_the_old_terminal_line_height_default_to_preserve_tui_block_graphics`
   failed:
   ```
   thread '...' panicked at crates/crowbar-core/src/settings/normalization.rs:417:9:
   got 1.2, want 1
   test result: FAILED. 0 passed; 1 failed
   ```
   Reverted.
3. **The workspace-keep-alive "rejects strings" asymmetry — made
   `normalize_workspace_keep_alive_minutes` parse a numeric string the way
   `normalize_ui_font_size`/`normalize_markdown_font_size` do.**
   `workspace_keep_alive_minutes_falls_back_to_default_for_nan_missing_or_a_string`
   failed:
   ```
   thread '...' panicked at crates/crowbar-core/src/settings/normalization.rs:419:9:
   got 45, want 10
   test result: FAILED. 0 passed; 1 failed
   ```
   Reverted.
4. **Theme registry migration — `normalize_theme`'s membership check
   short-circuited to always accept the value.**
   `coerces_a_theme_id_the_registry_does_not_know_to_the_default_theme`
   failed:
   ```
   thread '...' panicked at crates/crowbar-core/src/settings/normalization.rs:713:9:
   assertion `left == right` failed
     left: "terra"
    right: "crowbar"
   test result: FAILED. 0 passed; 1 failed
   ```
   Reverted.
5. **The documented §4-item-1 finding itself — `FILE_TREE_INDENT_SIZE_NON_FINITE_FALLBACK`
   20.0 → 16.0** (simulating someone "fixing" it to match the schema
   default). `file_tree_indent_size_non_finite_fallback_is_20_not_the_schema_default_of_16`
   failed:
   ```
   thread '...' panicked at crates/crowbar-core/src/settings/normalization.rs:417:9:
   got 16, want 20
   test result: FAILED. 0 passed; 1 failed
   ```
   Reverted — this one specifically proves the §4 finding is load-bearing
   test coverage, not prose.

All five mutations were made and reverted with `sed`/`cp`+`mv` against a
single source file, verified clean afterward (`git status`, plus a `grep`
for each mutated literal/expression showing the original value restored).
One caching gotcha worth recording: after the third `mv`-based revert,
`cargo test` initially reported the *mutated* result even though the
on-disk source was already correct — `mv` from a `cp`-made backup carries
the backup's original (older) mtime, so `cargo`'s mtime-based staleness
check considered the source unchanged and served a stale compiled test
binary. `touch`-ing the reverted files before the next `cargo test` fixed
it; every mutation from #4 onward used `cp`/`sed -i` in place instead
of `mv`-from-backup for exactly this reason.

## 7. What was not ported, and why (all CONDITIONAL or out of scope per §4)

- **`types/search.ts`, `config/search-index.ts`, `lib/settings-row-search.ts`,
  `lib/settings-tab-visibility.ts`** — the settings-search feature.
  CONDITIONAL (Settings dialog search box) and presentation (387 lines of
  static label/keyword copy).
- **`lib/settings-import-export.ts`, `lib/settings-download.ts`,
  `lib/diagnostics-export.ts`, `utils/theme-upload.ts`** — all CONDITIONAL,
  gated behind specific Settings dialog tabs/buttons. See §0:
  `settings-import-export.ts` is the survey's own "ninth file" and is
  CONDITIONAL, not LIVE — correctly out of this item's scope.
- **`lib/settings-persistence.ts`** — the `localStorage`-backed shim.
  Explicitly out of scope per the brief (D6; §9.3's `/v0/settings/ui` is
  the daemon-side replacement).
- **`lib/settings-effects.ts`, `lib/appearance-bootstrap.ts`** — DOM/CSS
  application (theme class toggling, pre-hydration FOUC-prevention cache).
  Out of scope per the brief; §6.1's sealed `Theme` struct replaces the
  mechanism entirely, not just the DOM half.
- **`lib/settings-bootstrap.ts`** — boot orchestration gluing persistence +
  normalization + effects. Phase 4 wiring, not logic; its one embedded rule
  (derive `theme` from `syncSystemTheme` + `matchMedia`) is `window`-coupled
  in the TS source and not ported.
- **`store.ts`, `stores/agent-providers-store.ts`, `stores/font-store.ts`,
  `stores/types/font.ts`** — zustand stores. Phase 4 (`crowbar-state`'s
  `Entity<T>`).

## 8. Coverage

`cargo llvm-cov -p crowbar-core`: **100.00% line coverage over 2531 lines**
(up from **1882 lines, 100.00%** before this item — the P3.70 keymap
baseline). This item added 649 covered lines, all at 100.00%: `defaults.rs`
89, `font_family.rs` 82, `markdown_font_size.rs` 59, `normalization.rs`
327, `raw_value.rs` 5, `types.rs` 21, `ui_font_size.rs` 66 (executable lines
as `cargo llvm-cov` counts them — smaller than each file's `wc -l`, which
also counts doc comments and blank lines).

Region coverage is 99.51% workspace-crate-wide (19 missed regions, 10 of
them in `normalization.rs`) while line coverage is 100.00% — the same shape
`keymap/chord.rs` (96.62% regions / 100.00% lines) and `workspace/scope.rs`
(99.72% regions / 100.00% lines) already have in this crate; not a new kind
of gap this item introduced. The three real (non-redundant-arm) region
gaps found while investigating — `resolve_available_font_family`'s
degenerate empty-fallback branch, three of
`external_editor_as_engine_override`'s four match arms, and
`migrate_theme_mode_from_sync_system_theme`'s "value is present" branch —
were closed with dedicated tests rather than left as an unexplained
percentage (see each file's `#[cfg(test)]` module for the added cases);
the remaining region misses are wildcard-arm bookkeeping (e.g.
`_ => RenderWhitespaceMode::None` sharing a source region with the removed
explicit `Some("none")` arm clippy flagged as redundant), not uncovered
behaviour.

231 tests in `crowbar-core`'s lib target — **171 before this item** (the
P3.70 keymap baseline) **-> 231 after**, 60 new, all in `settings::*`: 57
from this item's initial port (the branch-by-branch enumeration in §4-§5),
plus 3 more added after the first `cargo llvm-cov` run (99.33% line
coverage, 5 missed lines) surfaced three real uncovered branches — closed
with tests asserting the actual mapped value, not left as an unexplained
percentage (§9). Of the 60: 15 TS cases from `settings-normalization.test.ts`,
4 from `settings-normalization-theme.test.ts`, 6 from
`font-family-resolution.test.ts`, 6 from `markdown-font-size.test.ts`, 5
from `ui-font-size.test.ts` have a ported counterpart (36 TS cases total),
plus 24 authored — either "new: not exercised by the TS suite"
boundary/branch cases noted in each test module, or cases relocated from a
TS test that targeted a now-sealed field (§5). **A caveat on that 15,
stated precisely rather than implied:** two of `settings-normalization.test.ts`'s
15 cases (`'preserves font updates before persisting'`,
`'falls back for empty font updates'`) call `normalizeSettingValue` for
`fontFamily`/`terminalFontFamily`/`uiFontFamily`, which is a direct,
unconditional forward to `normalizeConfiguredFontFamily` with no
`normalization.rs`-specific logic in between — the exact function
`font-family-resolution.test.ts`'s own
`preserves_configured_font_names_that_may_exist_on_the_system` /
`falls_back_when_the_configured_font_is_empty` already exercise with the
same inputs. Those two TS cases have no *separate* Rust test in
`normalization.rs`; their behaviour is exercised via `font_family.rs`'s
suite instead of a redundant copy. Counted as "ported" because the
behaviour is genuinely tested, not because a duplicate exists in this file.
`typography.rs`/`defaults.rs`/`types.rs` had no TS test files to port from
at all (`typography-defaults.ts` and `default-settings.ts` have none;
`types/settings.ts` has none either) — all
of `defaults.rs`'s 2 tests and `types.rs`'s 4 tests are authored.

Full-workspace gates, run in the foreground: `cargo clippy --workspace
--all-targets -- -D warnings` clean; `cargo test --workspace` — **2331
passed, 0 failed** (up from the 2271-test trunk baseline plus this item's
60 new `crowbar-core` tests); `bash native/scripts/check-invariants.sh` —
7 of 7 invariants pass, including `cargo fmt --check` (run after this
item's files were formatted with `cargo fmt -p crowbar-core`).

## 9. What this pass found, restated plainly

Re-reading everything above for claims that outran their evidence, in the
spirit of this item's closing instruction:

- The 8-vs-9 reconciliation (§0) came back clean — no miscount to report
  there, unlike prior items on this project. Checked against both the
  survey's reconciliation table and its independent per-file Liveness
  table, which agree.
- Four real behavioural asymmetries in the original TS source were found by
  enumerating every branch by hand (§4) — none of these were flagged in
  `tier-a-denominator.md`'s own prose; all four are new findings from this
  item's own reading, not restatements of something the survey already
  said.
- The mutation-testing pass caught a real tooling gotcha (§6's `mv`/mtime
  note) that would have silently produced a false-positive "still failing"
  reading if not investigated — recorded rather than quietly worked around.
- The first coverage run was **not** 100% line coverage (99.33%, 5 lines
  missed) — three of those misses were real, uncovered behaviour (one
  degenerate branch in `font_family.rs`, three of four match arms in an
  external-editor-to-engine mapping, and one branch of the themeMode
  migration split), not padding-worthy noise. All three were closed with
  tests that assert the actual mapped value, not just "the function
  didn't panic." Stated here because shipping the first number without
  investigating what the 5 missing lines actually were would have been
  exactly the kind of gap this project's standing findings warn against.

[`types`]: ../crates/crowbar-core/src/settings/types.rs
[`types::Settings`]: ../crates/crowbar-core/src/settings/types.rs
[`types::CoreFeaturesState`]: ../crates/crowbar-core/src/settings/types.rs
[`types::FileTreeDensity`]: ../crates/crowbar-core/src/settings/types.rs
[`types::normalize_file_tree_density`]: ../crates/crowbar-core/src/settings/types.rs
[`typography`]: ../crates/crowbar-core/src/settings/typography.rs
[`defaults`]: ../crates/crowbar-core/src/settings/defaults.rs
[`defaults::default_settings`]: ../crates/crowbar-core/src/settings/defaults.rs
[`defaults::DEFAULT_THEME_ID`]: ../crates/crowbar-core/src/settings/defaults.rs
[`normalization`]: ../crates/crowbar-core/src/settings/normalization.rs
[`normalization::normalize_settings`]: ../crates/crowbar-core/src/settings/normalization.rs
[`normalization::normalize_editor_engine`]: ../crates/crowbar-core/src/settings/normalization.rs
[`normalization::migrate_theme_mode_from_sync_system_theme`]: ../crates/crowbar-core/src/settings/normalization.rs
[`normalization::normalize_theme_mode_value`]: ../crates/crowbar-core/src/settings/normalization.rs
[`font_family`]: ../crates/crowbar-core/src/settings/font_family.rs
[`markdown_font_size`]: ../crates/crowbar-core/src/settings/markdown_font_size.rs
[`ui_font_size`]: ../crates/crowbar-core/src/settings/ui_font_size.rs
[`raw_value`]: ../crates/crowbar-core/src/settings/raw_value.rs
[`raw_value::RawNumber`]: ../crates/crowbar-core/src/settings/raw_value.rs
