# Tier A denominator — `crowbar-core`, measured from the React source

Companion to QUEUE.md's "The real Tier B denominator" — same rigour, different
target. Spec §16 Phase 3: *"Tier A (`core`, `proto`, `client`, theme tokens —
gated by ported tests) and Tier B."* `crowbar-core`'s `Cargo.toml` claims **"all
Crowbar domain logic: git model, diff algebra, keymap resolution, settings
schema, file-tree model, workspace scoping, review threads."** Today it is
`color.rs` + `lib.rs`, 349 lines, 100% coverage over a crate that holds none of
that domain logic (QUEUE.md, 2026-08-03).

**Status: IN PROGRESS. This skeleton is committed first per the interruption
protocol; each area below is filled in and committed as it completes.**

Method, per area: (1) where the logic lives today in `web/src`, with line
counts: (2) what if anything is already ported into `crowbar-proto` /
`crowbar-client`; (3) whether it is expressible with zero reference to a view,
store or framework (gpui-free, D2); (4) the bucket — Tier A core / Phase 4
state / already done / presentation / out of scope; (5) existing test files
and case counts, since §16 gates Tier A on ported tests.

---

## 1. Git model

### Where it lives

`web/src/features/git/`, non-component, non-store:

| file | lines | shape |
|---|---|---|
| `types/git-types.ts` | 78 | `GitFile`, `GitStatus`, `GitCommit`, `GitDiffLine`, `GitDiff`, `GitHunk`, `GitRemote`, `GitStash`, `GitBlame*` — hand-written DTOs |
| `types/git-diff-types.ts` | 41 | `ParsedHunk` (dead — see below), `MultiFileDiff`, two component-prop interfaces |
| `lib/branch-action.ts` | 49 | `resolveBranchAction` — pure decision function |
| `utils/git-status-to-changed-files.ts` | 45 | status → sidebar-tree projection, dedup |
| `utils/review-file-summary-to-git-diff.ts` | 41 | review-summary → sidebar-tree projection |
| `utils/build-git-folder-tree.ts` | 57 | flat `GitFile[]` → folder tree |
| `utils/git-diff-helpers.ts` | 11 | `getFileStatus` (logic) + `getImgSrc` (presentation, mixed in one file) |
| `utils/normalize-diff.ts` | 38 | defends against a stale-persisted-shape bug (see below) |
| `utils/diff-buffer-path.ts` | 24 | parses the synthetic `diff://…` buffer-path scheme |
| `utils/diff-search.ts` | 72 | regex search over reconstructed unified diff text |
| `utils/git-diff-cache.ts` | 122 | in-memory TTL cache, keyed `repo:file:staged` |
| `lib/patch-window.ts` | 244 | windowed-review materialisation planner (pure by design) |
| `components/diff/review-code-view.tsx` | 1,179 | **component file, but 368 of the 1,179 lines (roughly lines 93–460) are pure functions**: `partitionReviewFiles`, `buildPlaceholderFileDiff`, `distributeContext`, `buildPlaceholderHunks`, `buildTailHunk`, `trimToPatchCap`, `reserveAtMost`, `parseSingleFilePatch`, `patchCacheKey` |

**822 lines of `utils/`+`lib/`, plus ~368 pure lines embedded in a 1,179-line
component** = **~1,190 lines of git/diff logic outside stores and components proper.**

### ‼️ Finding: `features/git/utils/git-diff-parser.ts` does not exist

§10.1 names it explicitly: *"unless the daemon already returns unified diff, in
which case `features/git/utils/git-diff-parser.ts` ports directly and no
algorithm is needed. **Check first.**"* I checked. No file by that name exists
anywhere in `web/src`, and no hand-rolled unified-diff string parser exists
either. What actually happens:

1. The daemon **does** return structured diff data — `GitDiff.lines:
   GitDiffLine[]` — never a raw patch string for the sidebar/status path. See
   `normalize-diff.ts`'s whole reason for existing: a persisted tab's stale
   payload once arrived with `lines` missing, so the fix is a defensive
   reshape, not a parser.
2. Where a **raw patch string** does need parsing (the Branch Review surface,
   which streams patch text lazily), the app delegates to the third-party
   `@pierre/diffs` library's `parsePatchFiles` — see `parseSingleFilePatch` in
   `review-code-view.tsx:400`, a 10-line wrapper around it, not a hand-rolled
   parser.

So §10.1's conditional resolves to "no algorithm needed" — correctly — but the
*mechanism* it names is wrong: there is no `git-diff-parser.ts` to port, and the
real patch-string parsing the app does today is a third-party dependency
(`@pierre/diffs`, being replaced wholesale by `crowbar-diff` per §5.2) rather
than portable first-party code.

### What is genuine, portable git-model logic

- **`resolveBranchAction`** (`lib/branch-action.ts`) — a precedence-ordered pure
  decision function (`commit` > `resolve` > `pull-request` > `merge` >
  `sync-only`) over `{hasUncommitted, hasParent, canMergeLocally, status, ahead,
  behind}`. Textbook Tier A: no DOM, no store, no framework.
- **`gitStatusToChangedFiles`** / **`reviewFilesSummaryToChangedFiles`** — two
  independent projections from two different daemon summary shapes into the
  same `GitDiff[]` sidebar-tree shape, each with its own status-interpretation
  rules (`'untracked' → is_new`, staged/uncommitted flag handling). Real,
  small, duplicated-but-distinct git-status semantics.
- **`buildGitFolderTree`** — flat changed-file list → folder tree (path
  segmentation, dedup, sort). Same shape as file-tree model (§5 below); this is
  the git-scoped variant of it.
- **`getFileStatus`** in `git-diff-helpers.ts` — a third, smaller
  restatement of the same is_new/is_deleted/is_renamed → label mapping
  already done twice above. Three near-duplicate implementations of "classify
  a file's change kind" is itself a finding: a single `crowbar-core` type
  should collapse all three.
- **`partitionReviewFiles`** (in `review-code-view.tsx`) — classifies each
  changed file as diff/image/binary from outline + summary. Pure, gpui-free.
- **The placeholder-geometry algebra** (`buildPlaceholderFileDiff`,
  `buildPlaceholderHunks`, `distributeContext`, `buildTailHunk`,
  `trimToPatchCap`, `reserveAtMost`) — genuinely the closest thing to "diff
  algebra" in the app: it reconstructs per-hunk row-count estimates from
  outline geometry (`oldStart/oldLines/newStart/newLines`) and the file's ±
  summary counts, entirely without line content. **But its input/output types
  (`Hunk`, `FileDiffMetadata`) are `@pierre/diffs` library types**, which are
  not carried into the port (§5.2 replaces `@pierre/diffs` with native
  `crowbar-diff`). The *algorithm* — how to distribute a context estimate
  across hunks bounded by `min(oldLines,newLines)`, how to trim/scale a
  placeholder to a patch cap — is portable in concept; the code is not,
  because its types belong to a library being deleted.

### What is not git-model logic

- **`git-diff-cache.ts`** — a hand-rolled TTL/LRU-ish in-memory cache keyed on
  `repo:file:staged`, with content-hash invalidation. This is exactly the
  caching architecture §9.3/D6 replaces: *"against a local daemon on a unix
  socket there is no latency to hide … `Entity<T>` fed by WS is the cache."*
  Not domain logic — a performance shim for a network-latency problem the
  native client does not have. **Out of scope, cite D6/§9.3.**
- **`patch-window.ts`'s `planWindow`** — deliberately documented as "no React,
  no fetch, no timers," and it is the single most rigorously pure file in the
  whole area (25 ported-able test cases). But it is a **viewport
  materialisation scheduler** (what to fetch/evict given scroll position and
  memory budgets), not diff algebra. §4.2 gives `crowbar-diff` its own
  logic partition (§12: `"diff(logic)" ≥98%`) precisely for pure logic that is
  diff-*rendering*-adjacent rather than diff-*structure* domain model. This
  belongs to `crowbar-diff`, not `crowbar-core` — an important distinction the
  brief's bucket list does not spell out: "diff algebra" in `crowbar-core`
  means hunk/line structure, not the windowing that consumes it.
- **`diff-search.ts`** — regex search over reconstructed unified text for the
  review surface's search bar. Pure and gpui-free, but it is view-search logic
  (imports `features/editor/utils/search`), the same `crowbar-diff`-logic
  bucket as `patch-window.ts`, not core git-model/diff-algebra.
- **`diff-buffer-path.ts`** — parses a synthetic `diff://staged/<path>` buffer
  identifier used for editor-tab addressing. Pure, but it's tab/buffer
  identity logic (editor/tabs feature), not git model.
- **`ParsedHunk`** (in `git-diff-types.ts`) is declared and never constructed
  anywhere in `web/src` — dead type.
- **`ImageContainerProps`/`ImageDiffViewerProps`** (same file) are component
  prop types — presentation, belongs with the component.
- **`getImgSrc`** (`git-diff-helpers.ts`) — formats a `data:` URI; presentation.

### Already done in `crowbar-proto`

`native/crates/crowbar-proto/src/generated/domain_git.rs` (316 lines) **already
has generated DTOs** for `Branch`, `Commit`, `Stash`, `Identity`,
`ConflictHunk`/`ConflictResolution`, `DiffLine`/`DiffLineType`, `FileDiff`,
`FileOutline`, `GitFileStatus`, `Hunk`, `HunkShape`, `MergeStrategy`,
`MultiFileDiff`, `ReviewFileSummary`, `SearchHit` — a near-exact match for
`web/src/features/git/types/git-types.ts` and `git-diff-types.ts` (119 lines
combined). **The two hand-written TS type files are not new Tier A work — they
duplicate what `crowbar-proto` already generated from the Go handlers (§9.2).**
The domain logic (branch-action, the two changed-files projections,
folder-tree building, the placeholder algebra's *concept*) is the part with no
existing counterpart.

### gpui-free?

Yes, for the genuine-logic set (`branch-action.ts`, both `*-to-changed-files`
projections, `build-git-folder-tree.ts`, `partitionReviewFiles`, the
placeholder-geometry functions rewritten against `crowbar-proto`'s `Hunk`
instead of `@pierre/diffs`'s). No DOM, no store, no React import in any of
them except `diff-buffer-path.ts` and `diff-search.ts`, which are not core
material anyway (see above).

### Tests

| test file | cases | covers |
|---|---|---|
| `utils/build-git-folder-tree.test.ts` | 19 | folder-tree building |
| `utils/normalize-diff.test.ts` | 6 | stale-shape defence |
| `utils/review-file-summary-to-git-diff.test.ts` | 6 | summary → sidebar projection |
| `utils/git-status-to-changed-files.test.ts` | 4 | status → sidebar projection |
| `utils/diff-search.test.ts` | 7 | (crowbar-diff logic, not core) |
| `lib/branch-action.test.ts` | 6 | `resolveBranchAction` |
| `lib/patch-window.test.ts` | 25 | (crowbar-diff logic, not core) |
| `components/diff/review-code-view.test.tsx` | 15 total, **5** (`partitionReviewFiles` ×2, `buildPlaceholderFileDiff` ×3) test the pure functions; the other 10 are component/windowing behaviour |

**Git-model Tier A test total: 19+6+6+4+6+5 = 46 cases across 6 areas** (folder
tree, normalize-diff, two projections, branch-action, placeholder algebra).
Zero test files exist for `git-diff-helpers.ts`'s `getFileStatus` — it is
exercised only incidentally through the two projection tests above.

## 2. Diff algebra

**Finding: as a distinct area, "diff algebra" barely exists as first-party
code.** The daemon does the actual diffing (git itself, via the Go layer) and
returns structured `FileDiff`/`Hunk`/`DiffLine` shapes already generated into
`crowbar-proto` (`domain_git.rs`). §10.1's own conditional — "unless the daemon
already returns unified diff, in which case [it] ports directly and no
algorithm is needed" — resolves to **no algorithm needed**, confirmed above.
What the React app implements instead, under the `features/git/` files
surveyed in §1, are three things that are easy to mistake for "diff algebra"
but are not the same claim:

1. **File-status classification** (is_new/is_deleted/is_renamed → label) —
   genuine, tiny, git-model logic, counted in §1.
2. **Placeholder hunk-geometry estimation** for the windowed review renderer —
   real pure math, but its types belong to `@pierre/diffs` (being deleted) and
   its *purpose* is virtualiser sizing, i.e. `crowbar-diff`-crate logic, not
   `crowbar-core`.
3. **Viewport windowing/materialisation** (`patch-window.ts`) and **diff-text
   search** (`diff-search.ts`) — pure, well-tested, but `crowbar-diff`-crate
   logic, not `crowbar-core`.

So `crowbar-core`'s "diff algebra" is real but small: essentially the
file-status/change-kind classification already counted under git model, plus
whatever hunk/line data-shape work is needed to consume `crowbar-proto`'s
`FileDiff`/`Hunk`/`DiffLine` types directly (largely already solved by
`crowbar-proto` existing). The bulk of what *looks* like diff algebra in the
React app — windowing, search, placeholder sizing — belongs to `crowbar-diff`'s
own logic partition per §4.2/§12, not to `crowbar-core`. **This is the report's
first correction to the brief's bucket list**: "diff algebra" in the crate
description undersells how much of the React app's diff-adjacent pure logic is
actually scoped to a *different* crate (`crowbar-diff`) that also has a
gpui-free logic gate.

## 3. Keymap resolution

### Where it lives

All of it is under `web/src/features/keymaps/` (733 lines total, matching §3.1
exactly — no keymap logic found anywhere outside this directory):

| file | lines | shape |
|---|---|---|
| `types.ts` | 52 | `Command`, `CommandCategory`, `KeymapPreset(Id)`, `KeymapOverrides`, `EffectiveBinding` — the schema |
| `registry.ts` | 220 | `COMMANDS: Command[]` (19 commands) + `getCommand`, `CATEGORY_ORDER` — static data + lookup |
| `defaults/keybinding-presets.ts` | 49 | `KEYMAP_PRESETS` (`default`, `compact`) + `getPreset`, `isKeymapPresetId` |
| `utils/chord.ts` | 124 | chord grammar: parse/stringify/normalize/format + 2 `KeyboardEvent`-consuming functions |
| `utils/effective-keymaps.ts` | 71 | **the resolution algorithm**: `resolveBinding`, `getEffectiveBindings`, `getEffectiveChordMap`, `findConflictingCommands` |
| `stores/store.ts` | 100 | zustand store: active preset + user overrides, localStorage-persisted |
| `hooks/use-effective-keymap.ts` | 17 | thin `useMemo` wrapper over `getEffectiveChordMap` |
| `hooks/use-command-shortcut.ts` | 4 | **stub** — `return undefined` |
| `hooks/use-save-keyboard.ts` | 31 | `useEffect` + `window.addEventListener('keydown', …)` |
| `hooks/use-sidebar-tab-keyboard.ts` | 40 | same pattern |
| `hooks/use-workspace-switcher-keyboard.ts` | 25 | same pattern |

### What is genuine, portable keymap-resolution logic

- **`types.ts` + `registry.ts` + `keybinding-presets.ts`** — the schema itself:
  a finite command list with default chords, categories, and a
  precedence-ordered preset system. 100% data + pure lookups. This is
  literally "keymap resolution"'s input model.
- **`chord.ts`'s grammar functions** — `parseChord`, `stringifyChord`,
  `normalizeChord`, `formatChord` are pure string algebra over a documented
  grammar (`[mod+][shift+][alt+]<key>`). Fully gpui-free.
- **`effective-keymaps.ts`** — the actual resolution algorithm the crate
  description names: `resolveBinding` merges default → preset → user override
  by precedence, normalizing every chord so comparison is canonical;
  `findConflictingCommands` does conflict detection across the resolved set.
  Pure, gpui-free, and the smallest, most literal match to "keymap resolution"
  in the whole survey.

### What is entangled or not core

- **`chord.ts`'s `chordFromEvent`/`eventMatchesChord`** take a DOM
  `KeyboardEvent` directly. The grammar they call into (`parseChord`,
  `stringifyChord`) is portable; the event-field extraction
  (`e.metaKey`/`e.ctrlKey`/`e.shiftKey`/`e.altKey`/`e.key`) is not — GPUI
  delivers its own `KeyDownEvent`/`Modifiers` shape (see the gpui skill), so
  this is a reimplementation-at-the-boundary, not a port, of 2 of the file's 6
  exports.
- **`store.ts`** — a zustand store persisting to `localStorage` under
  `crowbar:settings:keybindingPreset`/`…UserOverrides`. Reactive-state +
  persistence shell: Phase 4 (`Entity<T>`), and the persistence mechanism
  itself is D6 territory (deleted, not ported — see §4 below for where user
  keybinding overrides would need to land in the daemon-side `/v0/settings/ui`
  scheme if kept at all).
- **The four hooks** (`use-effective-keymap`, `use-save-keyboard`,
  `use-sidebar-tab-keyboard`, `use-workspace-switcher-keyboard`) are
  `useEffect` + `window.addEventListener('keydown', …)` wiring that dispatches
  on a resolved chord. This is not "keymap resolution" so much as the
  reactive-subscription layer §7 governs — and in GPUI it doesn't port at all
  in this shape: GPUI has its own native action/keybinding dispatch system
  (see the gpui skill), so this glue is replaced, not translated.
  `use-command-shortcut.ts` is a dead stub (4 lines, returns `undefined`).

### gpui-free?

The schema (`types.ts`, `registry.ts`, `keybinding-presets.ts`) and the
resolution algorithm (`effective-keymaps.ts`) — yes, entirely. The chord
grammar (`parseChord`/`stringifyChord`/`normalizeChord`/`formatChord`) — yes.
The event-matching half of `chord.ts` — no, tied to `KeyboardEvent`; the
*concept* (compare mod/shift/alt/key against a parsed chord) survives, the
code does not verbatim.

### Already done in `crowbar-proto`/`crowbar-client`

None. No keymap-related type appears in `crowbar-proto`'s generated set — this
is pure frontend-local state with no daemon wire representation today (user
overrides live in `localStorage`, not behind any `/v0/*` endpoint).

### Tests

| test file | cases | covers |
|---|---|---|
| `chord.test.ts` | 7 | chord grammar |
| `registry.test.ts` | 9 | `COMMANDS` data (pinned chord assignments) |
| `hooks/use-sidebar-tab-keyboard.test.ts` | 7 | hook/DOM wiring, not core |
| `hooks/use-save-keyboard.test.ts` | 6 | hook/DOM wiring, not core |
| `hooks/use-workspace-switcher-keyboard.test.ts` | 4 | hook/DOM wiring, not core |

**‼️ Finding: `effective-keymaps.ts` — the file that most literally *is*
"keymap resolution" — has zero dedicated tests.** No test file references
`resolveBinding`, `getEffectiveBindings`, `getEffectiveChordMap`, or
`findConflictingCommands` anywhere in `web/src/__tests__`. Neither does
`keybinding-presets.ts` or `store.ts`. Since §16 gates Tier A on *ported*
tests, this area's most important single file arrives at the port with no
test suite to port — new tests would have to be authored, not translated, a
materially different (and more expensive) kind of Tier A work than the areas
that do have a suite already.

**Keymap-resolution Tier A test total: 7 (chord) + 9 (registry) = 16 cases.**
The 17 hook-test cases are Phase 4/glue, not core.

## 4. Settings schema

### Where it lives

`web/src/features/settings/` outside `components/`:

| file | lines | shape |
|---|---|---|
| `types/settings.ts` | 81 | `Settings` interface — **~50 fields**, the schema itself |
| `types/feature.ts` | 3 | `CoreFeaturesState` |
| `types/search.ts` | 20 | `SettingSearchRecord`/`SearchResult`/`SearchState` — presentation |
| `config/default-settings.ts` | 98 | `defaultSettings` + `getDefaultSetting`/`getDefaultSettingsSnapshot` |
| `config/typography-defaults.ts` | 25 | font/size constants |
| `config/search-index.ts` | 387 | static UI-copy data for settings search (labels/descriptions/keywords) — presentation |
| `lib/settings-normalization.ts` | 249 | **validation/clamping/migration for ~15 fields** |
| `lib/font-family-resolution.ts` | 40 | font-family parse/normalize/resolve-against-available |
| `lib/markdown-font-size.ts` | 26 | clamp/snap one numeric field |
| `lib/ui-font-size.ts` | 32 | clamp/snap/scale one numeric field |
| `lib/settings-import-export.ts` | 75 | versioned export envelope, import validation |
| `lib/settings-download.ts` | 39 | `buildSettingsExportFile` (pure) + `downloadSettingsFile` (DOM) |
| `lib/settings-bootstrap.ts` | 55 | orchestrates persistence+normalization+side-effects at startup |
| `lib/settings-persistence.ts` | 130 | **localStorage-backed** store shim, D6 territory |
| `lib/settings-effects.ts` | 198 | DOM class/attribute application (theme, transparency) |
| `lib/appearance-bootstrap.ts` | 197 | pre-hydration FOUC-prevention cache, DOM-applied |
| `lib/settings-row-search.ts` | 16 | string match, `ReactNode`-typed param |
| `lib/settings-tab-visibility.ts` | 16 | tab filtering by search match — presentation |
| `lib/diagnostics-export.ts` | 18 | Tauri IPC command wrapper (§3.5) |
| `utils/theme-upload.ts` | 154 | theme-file validation + CSS-variable generation |
| `store.ts` | 171 | zustand store |
| `stores/agent-providers-store.ts` | 85 | zustand store, async fetch |
| `stores/font-store.ts` | 156 | zustand store, async fetch + cache |
| `stores/types/font.ts` | 6 | `FontInfo` type |

**2,277 lines total** across these 22 files (matches `wc -l`, measured
directly).

### What is genuine settings-schema logic

- **`types/settings.ts`** — the `Settings` interface itself: ~50 fields
  spanning general/editor/terminal/UI/theme/layout/language/file-tree
  settings. This is exactly "settings schema" as named.
- **`config/default-settings.ts`** — the default-value table + accessors.
- **`lib/settings-normalization.ts`** — the validation half of "settings
  schema + validation": per-field clamping (`normalizeEditorLineHeight`,
  `normalizeFileTreeIndentSize`, `normalizeWorkspaceKeepAliveMinutes`),
  enum-membership checks (`renderWhitespace`, `editorEngine`,
  `externalEditor`), a dead-theme-id fallback, and cross-field rules (e.g.
  `externalEditor !== 'none'` overrides `editorEngine`; a `custom` engine with
  no command string falls back to `monaco`). 249 lines, entirely pure,
  entirely gpui-free except one call into `themeRegistry.getTheme` (itself a
  pure lookup, not a component).
- **`lib/font-family-resolution.ts`, `markdown-font-size.ts`,
  `ui-font-size.ts`** — smaller, single-field validators the normalization
  file composes. Pure.
- **`lib/settings-import-export.ts`** — versioned export/import envelope
  (`format`/`version`/`exportedAt`), and import validation that re-runs
  `normalizeSettings` over whatever the file contained. Pure (`isRecord`,
  `pickSettings`, `getSettingsCandidate`, `createSettingsExportPayload`,
  `parseSettingsImportJson`) — genuinely "safe to unit test without a DOM," as
  `settings-download.ts`'s own comment for the sibling function puts it.
- **`lib/settings-download.ts`'s `buildSettingsExportFile`** — the
  serialize-to-JSON half is pure; `downloadSettingsFile` (Blob/anchor-click)
  is not and is out of scope regardless (native file-save is a platform
  concern, §3.5-adjacent).

### What is not settings-schema logic

- **`lib/settings-persistence.ts`** — a `localStorage`-backed key/value shim
  under the `crowbar:settings:` prefix, with a `Store` interface
  (`get`/`set`/`save`/`onKeyChange`) mimicking Tauri's old settings-store API.
  **This is exactly what D6 deletes.** §9.3's `GET/PUT /v0/settings/ui`
  (already built, QUEUE.md item 0.6, verified live) is the daemon-side
  replacement — but note it stores **opaque JSON**, so it does not itself
  carry the `Settings` schema; the schema still has to exist client-side to
  give that opaque blob a shape. **Persistence mechanism: out of scope (D6).
  Schema itself: still needed, and is the Tier A content above.**
- **`lib/settings-effects.ts`, `lib/appearance-bootstrap.ts`** — both are
  `document`/`window`/CSS-custom-property/class-list manipulation: applying a
  theme by toggling `.dark`, setting `--app-font-family` etc., syncing
  `matchMedia('(prefers-color-scheme: dark)')`, and (in
  `appearance-bootstrap.ts`) a pre-React-hydration flash-of-unstyled-content
  cache read/applied before the app mounts. **This entire mechanism is a
  webview artifact.** A native GPUI window has no FOUC to prevent (state is
  resolved before the first frame paints) and no CSS custom properties to
  toggle (§6.1's sealed `Theme` struct replaces it entirely). Out of scope —
  not a port target, not even conceptually; it is the exact class of problem
  §2 of the spec cites as the motivation for the rewrite.
- **`lib/settings-bootstrap.ts`** — orchestration gluing
  persistence+normalization+effects at startup; Phase 4 wiring. Contains one
  small embedded rule (derive `theme` from OS `prefers-color-scheme` when
  `syncSystemTheme` is on) that could be extracted as pure logic but isn't
  today — currently inline and `window.matchMedia`-coupled.
- **`lib/settings-row-search.ts`, `settings-tab-visibility.ts`,
  `config/search-index.ts`** — the settings-search feature: 387 lines of
  static label/description/keyword copy plus two small string-match/filter
  functions, one of which (`settingRowMatchesQuery`) types its `description`
  parameter as `ReactNode`. **Presentation, belongs with the settings-search
  component**, not domain logic — this is squarely the brief's fourth bucket
  ("formatting a label" — here, indexing one).
- **`utils/theme-upload.ts`** — validates an uploaded theme JSON file, but
  its output (`convertNewFormatTheme`) manufactures literal CSS custom
  property strings (`--color-${key}`) from user data. This is precisely what
  §6.1's sealed token types exist to prevent at compile time
  ("a worker agent cannot write `rgb(0x1e1e1e)` at a call site"): the
  *validation* logic (format detection, required-field checks) is portable,
  the *output shape* is not — it would need to construct `crowbar-ui::Color`
  values via `seal`, not CSS strings. Overlaps with the theme-tokens area
  below.
- **`lib/diagnostics-export.ts`** — a direct Tauri IPC command wrapper
  (`__TAURI_INTERNALS__.invoke('diagnostics_export')`). §3.5 already assigns
  `diagnostics_export` to `crowbar-platform`, not `crowbar-core`.
- **`store.ts`, `stores/agent-providers-store.ts`, `stores/font-store.ts`** —
  three zustand stores (search-run orchestration, async provider fetch with
  request-race guarding, async font enumeration with a 24h cache). Phase 4.
- **`types/feature.ts`, `types/search.ts`** — trivial/presentation-adjacent
  types, not schema in the crate-description sense.

### gpui-free?

The schema + normalization + import/export set — yes, fully, once
`themeRegistry.getTheme` is confirmed pure (it is a registry lookup, not a
component). Everything else in the directory either touches `document`/
`window`/`localStorage` directly or is reactive-state/Phase-4 material.

### Already done in `crowbar-proto`/`crowbar-client`

None found. `Settings` has no counterpart in `crowbar-proto`'s generated set —
unsurprising, since today it is never sent to the daemon as a typed payload
(only as opaque JSON via the new `/v0/settings/ui`, and even that endpoint
didn't exist before Phase 0 item 0.6). This is genuinely new Tier A surface,
not a duplicate of already-generated DTOs (contrast with git model, §1, where
the type files mostly *were* duplicates).

### Tests

| test file | cases | covers |
|---|---|---|
| `settings-normalization.test.ts` | 15 | `normalizeSettings`/`normalizeSettingValue` |
| `settings-normalization-theme.test.ts` | 4 | theme-id fallback specifically |
| `ui-font-size.test.ts` | 5 | `ui-font-size.ts` |
| `lib/markdown-font-size.test.ts` | 6 | `markdown-font-size.ts` |
| `font-family-resolution.test.ts` | 6 | `font-family-resolution.ts` |
| `settings-import-export.test.ts` | 3 | export/import envelope |
| `settings-download.test.ts` | 2 | `buildSettingsExportFile` (+DOM path) |
| `settings-tab-visibility.test.ts` | 4 | (presentation, not core) |
| `config/search-index.test.ts` | 4 | (presentation, not core) |
| `lib/settings-effects.test.ts` | 4 | (DOM, not core) |
| `lib/diagnostics-export.test.ts` | 3 | (platform IPC, not core) |
| `stores/agent-providers-store.test.ts` | 12 | (Phase 4, not core) |
| `stores/font-store.test.ts` | 2 | (Phase 4, not core) |
| `components/*` (3 files) | 19 | component rendering, not core |

**Settings-schema Tier A test total: 15+4+5+6+6+3+2(partial) ≈ 39–41 cases**
across 6–7 files (schema validation, both single-field validators, export
envelope). This is the largest ported-able test base of any of the seven
areas measured so far.

## 5. File-tree model

### ‼️ Methodological note: a nested nearly-duplicate directory tree

`features/file-explorer/{lib,stores,utils}/*.ts` are **all 1-line re-export
shims** (`export * from '../file-explorer/lib/X'`) pointing at the real
content under `features/file-explorer/file-explorer/{lib,stores,utils}/*.ts`.
`ls`-driven counting would double the real total; every line count below is
from the real (nested) files only. This is the file-tree analogue of the
QUEUE.md lesson about not trusting a directory listing as scope.

### Where it lives

| file | lines | shape |
|---|---|---|
| `file-explorer/lib/visible-file-tree-rows.ts` | 238 | flatten nested tree → visible virtualised rows, given expand state; search/filter with ancestor-expansion; sticky/guide ancestor computation |
| `file-explorer/lib/file-tree-gitignore.ts` | 237 | cascading `.gitignore` rule resolution across nested directories (via the `ignore` npm package) |
| `file-explorer/lib/file-tree-git-status.ts` | 122 | git-status → per-row decoration, with directory-level status propagated from the highest-priority child |
| `file-explorer/lib/env-template.ts` | 90 | `.env` file template generation + comment-preserving KEY=VALUE parsing (narrow, not really tree-shaped) |
| `file-explorer/lib/file-tree-density.ts` | 38 | density enum + `normalizeFileTreeDensity` (real) + `FILE_TREE_DENSITY_CONFIG` row-height/className map (presentation) |
| `file-explorer/utils/file-explorer-tree-utils.ts` | 96 | immutable tree mutations: `filterHiddenFiles`, `addNewItemToTree`, `removeEditingItemsFromTree`, `getAncestorDirectoryPaths` |
| `file-explorer/stores/file-explorer-tree-store.ts` | 146 | zustand store — expand/select/collapse state (**D2-named**, see below) |
| `file-explorer/stores/file-explorer-clipboard-store.ts` | 110 | zustand store — copy/cut/paste, network calls |
| `file-explorer/hooks/use-file-explorer-drag-drop.ts` | 315 | drag/drop DOM handlers |
| `file-explorer/hooks/use-file-explorer-inline-editing.ts` | 231 | inline rename/create DOM+state handlers |
| `file-explorer/hooks/use-file-explorer-gitignore.ts` | 79 | wires `file-tree-gitignore.ts` to store state |
| `file-explorer/hooks/use-file-explorer-sync.ts` | 50 | reconciliation glue |
| `file-explorer/hooks/use-file-explorer-visible-rows.ts` | 87 | wires `visible-file-tree-rows.ts` to store state |
| `file-explorer/hooks/use-file-explorer-context-menu.tsx` | — (not counted, component-adjacent) | |
| `features/files/lib/file-tree-api.ts` | 141 | transport (fetch calls) + `toAppFile` DTO mapping |
| `features/files/lib/file-upload.ts` | — | transport |
| `features/file-system/controllers/file-tree-utils.ts` | 22 | `findFileInTree` — depth-first lookup |
| `features/file-system/controllers/file-utils.ts` | — | mixed |
| `features/file-system/types/app.ts` | — | `FileEntry`/`AppFile` type definitions |

### What is genuine, portable file-tree-model logic

- **`visible-file-tree-rows.ts`** — the closest thing to a canonical
  "file-tree model" in the app: `buildVisibleFileTreeRows` (nested tree + expand
  set → flat visible-row list, with compact single-child-folder collapsing),
  `computeFileTreeSearchHits`/`filterFileTreeForFffHits` (name-substring search
  with ancestor auto-expansion), `getStickyAncestorRow(s)`/`getGuideAncestorRows`
  (breadcrumb/indent-guide support for virtualized rendering). All pure,
  gpui-free, no DOM.
- **`file-tree-gitignore.ts`** — real algorithmic weight: reference collection
  (`collectGitIgnoreFileReferences`), depth-ordered rule-set construction
  (`createFileTreeGitIgnoreRules`), and cascading ignore resolution that walks
  every ancestor directory before testing the target path itself
  (`isPathGitIgnoredByFileTreeRules`) — because a directory ignored by a parent
  rule ignores everything under it regardless of its own `.gitignore`. Uses the
  npm `ignore` package for pattern matching; Rust has a well-known equivalent
  (`ignore`, from ripgrep's author) that the port would reach for instead of a
  hand-rolled matcher.
- **`file-explorer-tree-utils.ts`** — immutable tree editing: filter/insert/
  remove/ancestor-walk, all recursive pure functions over `FileEntry[]`.
- **`features/file-system/controllers/file-tree-utils.ts`'s
  `findFileInTree`** — pure depth-first lookup, 22 lines, its own bug history
  documented in the file (used to unconditionally return null).
- **`file-tree-git-status.ts`'s logic half** — `createFileTreeGitStatusLookup`
  (propagate the highest-priority status up every ancestor directory, with an
  explicit priority table) and `resolveActiveWorkspaceGitStatus` (workspace-
  scope validity guard against a documented past bug where the comparison was
  always false). Real domain rules.
- **`file-tree-density.ts`'s `normalizeFileTreeDensity`/`isFileTreeDensity`** —
  small but genuine settings-schema-adjacent validation (already called from
  `settings-normalization.ts`, §4).

### What is presentation or not core

- **`file-tree-git-status.ts`'s `getFileTreeGitStatusDecoration`** returns a
  hardcoded Tailwind `colorClassName` string (`'text-git-modified-staged'`
  etc.) alongside the genuine `statusLetter`/`label` classification — a clean
  example of the brief's fourth bucket: real logic (which status wins,
  M/A/D/U/R) fused in the same function with presentation (which CSS class).
  §6.1's sealed `Color` tokens are exactly what replaces the class-string half.
- **`file-tree-density.ts`'s `FILE_TREE_DENSITY_CONFIG`** — row heights and
  Tailwind class strings per density mode. Presentation, belongs with the row
  component.
- **`env-template.ts`** — real, tested, pure logic (KEY=VALUE parsing with
  quote/escape/inline-comment awareness), but it is `.env`-file-content
  domain, not tree-shape domain. Doesn't fit any of the seven named areas
  cleanly; flagging it as an unclassified pure-logic pocket rather than
  forcing it into "file-tree model."
- **`file-explorer-tree-store.ts`** — a zustand store for
  expanded/selected-paths state. **This is the literal case D2 names as its
  own example**: *"Selection logic, tree-expansion state, and similar get
  pulled out of components into core."* The store's mutator bodies
  (toggle/select/expand-to-path/collapse-path/expand-all) are, underneath the
  `create/immer/combine` wrapper, pure `Set<string> → Set<string>`
  transitions — genuinely portable into `crowbar-core` as plain functions,
  with only the reactive-subscription shell going to `crowbar-state`. This is
  the one file in the whole survey the spec itself pre-classifies.
- **`file-explorer-clipboard-store.ts`** — network calls
  (`renameFileNode`/`copyFileNode`) and workspace lookup; Phase 4. One small
  embedded rule (a failed cut's entries stay staged, a successful cut's don't)
  is buried in an async function rather than factored out.
- **`use-file-explorer-drag-drop.ts`, `use-file-explorer-inline-editing.ts`,
  `use-file-explorer-gitignore.ts`, `use-file-explorer-sync.ts`,
  `use-file-explorer-visible-rows.ts`** — all `useEffect`/DOM-event/store-glue
  hooks (546 lines across the two largest). Phase 4/presentation-glue, not
  core, though the two "wire the pure lib function to store state" hooks
  (`use-file-explorer-gitignore.ts`, `use-file-explorer-visible-rows.ts`)
  confirm the lib functions above are already factored out cleanly.
- **`features/files/lib/file-tree-api.ts`** — transport (`fetchFileTree`,
  `createFileNode`, `renameFileNode`, `deleteFileNode`, `writeFileContent`):
  `crowbar-client` territory, not `crowbar-core`. Its `toAppFile` mapping
  function is the only logic-shaped piece, and it maps a DTO
  (`crowbar-proto` already generates `FileNode`, see below) to a display
  model — thin, not new domain logic.

### gpui-free?

Yes for the genuine set: `visible-file-tree-rows.ts`, `file-tree-gitignore.ts`,
`file-explorer-tree-utils.ts`, `file-tree-utils.ts`'s `findFileInTree`, and
the classification half of `file-tree-git-status.ts`. No DOM, no React, no
store import in any of them (the two hooks that *use* them are the
DOM/store-entangled layer, kept separate).

### Already done in `crowbar-proto`

`native/crates/crowbar-proto/src/generated/domain.rs` already has `FileNode`
(line 30) and `FileContent` (line 22) — generated DTOs matching
`FileNodeDTO`/`AppFile`. The transport-layer DTO shapes are done; the
tree-shape *algorithms* (visible-row flattening, gitignore cascade, status
propagation) have no counterpart and are the real Tier A content here.

### Tests

| test file | cases | covers |
|---|---|---|
| `file-tree-gitignore.test.ts` | 16 | gitignore cascade |
| `visible-file-tree-rows.test.ts` | 12 | row flattening/search/ancestors |
| `file-tree-git-status.test.ts` | 9 | status decoration + propagation |
| `file-tree-search-hits.test.ts` | 5 | (overlaps `visible-file-tree-rows`'s search half — separate file) |
| `file-explorer-tree-utils.test.ts` | 4 | immutable tree edits |
| `env-template.test.ts` | 3 | (not tree-shape, see above) |
| `file-explorer/hooks/*.test.{ts,tsx}` (2 files) | 13 | hook/DOM wiring, not core |
| `file-explorer-clipboard-store.test.ts` + `clipboard-paste-mapping.test.ts` | 11 | Phase 4, not core |
| `file-explorer-tree-item.test.tsx` | 6 | component, not core |
| `file-system/controllers/file-tree-utils.test.ts` | 5 | `findFileInTree` |
| `files/lib/file-tree-api.test.ts` + `files/file-tree-api.test.ts` | 15 | transport (**two test files for one source file — a mirror-structure drift CLAUDE.md's rule would not produce; likely one is stale**) |

**File-tree-model Tier A test total: 16+12+9+5+4+5 = 51 cases** across 6 files
(gitignore, row-building/search, status, tree-edit utils, tree lookup). The
largest single-area test base measured so far, ahead even of settings.

## 6. Workspace scoping

### Where it lives

Split across `web/src/lib/` and `web/src/features/workspace/lib/` — **not
concentrated in `features/workspace/` alone**, unlike the other six areas
which each live under one feature directory:

| file | lines | shape |
|---|---|---|
| `lib/workspace-scope.ts` | 87 | `WorkspaceScope{projectId,repoId,wsId}` + registry (`Map`) + route-path parse |
| `lib/workspace-scope-url.ts` | 28 | scope → REST base-path construction, home-workspace detection |
| `lib/workspace/resolve-root-path.ts` | 31 | active workspace → on-disk root path (store-entangled) |
| `lib/workspace/placeholder.ts` | 32 | placeholder-workspace detection + reason string |
| `lib/workspace/branch-workspace.ts` | 16 | branch → owning-workspace-id lookup within a repo |
| `features/workspace/lib/keep-alive-policy.ts` | 98 | **pure** retention/eviction policy for mounted workspaces |
| `features/workspace/lib/activation-freshness.ts` | 90 | warm-reactivation freshness ledger (borderline, see below) |
| `features/workspace/lib/home-workspace-resolver.ts` | 89 | async-fetch + cache + `useSyncExternalStore` — Phase 4 |
| `features/workspace/lib/external-buffer-sync.ts` | 90 | external-disk-change reconciliation for open editor buffers |
| `features/workspace/lib/reset-workspace-scoped-stores.ts` | 47 | store-reset-on-activation glue |
| `features/workspace/lib/open-file-content.ts` | 46 | fetch+decode+open orchestration |
| `features/workspace/lib/workspace-slot-style.ts` | 36 | DOM mount-strategy styling (webview artifact) |

**544 lines total** across these 12 files.

### What is genuine, portable workspace-scoping logic

- **`workspace-scope.ts`'s pure half** — `parseWorkspaceScopeFromPath` (regex
  match of `/ide/:projectId/:repoId/:wsId` into a `WorkspaceScope`) is exactly
  "workspace/path scoping" as named. The registry half (`_scopes: Map`,
  `setWorkspaceScope`/`recordWorkspaceScope`/`getWorkspaceScope`,
  module-level `_activeWorkspaceId`) is a **plain, framework-free lookup
  table** — no React, no zustand, no gpui — explicitly documented as living
  outside the store graph *on purpose*. This is a clean, ready-made example of
  the kind of small stateful registry D2 wants moved into `crowbar-core`
  directly rather than wrapped as `Entity<T>`.
- **`workspace-scope-url.ts`** — `workspaceBase(wsId)` (scope → REST path,
  including the home-workspace special case with no `repoId`) and
  `isHomeWorkspace(wsId)`. Real scoping rules (hierarchical URL construction,
  what makes a workspace a "home" workspace), gpui-free. Note: §4.2's
  dependency table lets `crowbar-core` depend on `crowbar-client`, so this is
  a legitimate `crowbar-core` responsibility even though its output is a URL
  path string that `crowbar-client` ultimately sends over the wire — no
  crate-graph cycle.
- **`lib/workspace/placeholder.ts`** — `isPlaceholderWorkspace` (a workspace
  with no on-disk worktree, keyed on a documented *absence-of-field* signal,
  not a status enum) and `placeholderReason` (precedence: live-holder path >
  daemon-recorded error > generic retry message). Pure, gpui-free, though
  `placeholderReason`'s output is a user-facing sentence — logic and copy
  fused in one function, the brief's fourth-bucket pattern again, but small
  enough that separating them buys little.
- **`lib/workspace/branch-workspace.ts`** — `findWorkspaceForBranch`: exact
  case-sensitive branch→workspace-id lookup within a repo, explicitly
  excluding the default/main-worktree workspace. Small, pure, real git×
  workspace domain rule.
- **`keep-alive-policy.ts`'s `planRetention`** — the strongest Tier A
  candidate in this area: explicitly injected-clock, pure, documented
  invariants (active workspace always retained; window + hard-cap eviction;
  deterministic tie-breaking). Same shape as `patch-window.ts`'s
  `planWindow` (§1/§2) — a resource-retention scheduler, not merely
  subscription glue. Unlike `patch-window.ts`, there is no sibling crate this
  obviously belongs to instead (no `crowbar-workspace` crate exists), so
  `crowbar-core` is the natural home.

### What is entangled, Phase 4, or presentation

- **`resolve-root-path.ts`** — reads directly from two other zustand stores'
  `.getState()` and `window.location.hash`. Not pure; Phase 4/glue, though the
  underlying question ("what is this view's on-disk root") is a real
  workspace-scoping *concept* whose implementation today is entangled.
- **`home-workspace-resolver.ts`** — async fetch, an in-memory cache Map, and
  `useSyncExternalStore` subscription plumbing. This is the SAME
  "dependency-free module singleton" architectural pattern as
  `workspace-scope.ts` (the file's own comment says so), but doing network
  I/O + subscription rather than pure parsing — squarely Phase 4.
- **`activation-freshness.ts` — a genuine ambiguity, flagged rather than
  silently resolved.** Every function in it is technically gpui-free (Maps,
  timestamps, no framework import), which by §7.1's literal text ("if a
  store's logic can be tested without gpui, it belongs in core") would argue
  Tier A. But the brief's own bucket-3 test says *"if its substance is
  subscription, invalidation or effect ordering, it is not Tier A"* — and
  this file's entire purpose is deciding whether a re-subscribe should re-seed
  or reuse cached data, i.e. invalidation timing for the reactive graph. I
  read it as Phase 4 under the brief's substance test, but flag the tension
  with §7.1's more permissive wording explicitly: **the spec's own two
  statements of this rule do not agree at the margin**, and
  `activation-freshness.ts` sits exactly on that margin.
- **`external-buffer-sync.ts`** — real rules (own-write-echo suppression,
  clean-vs-dirty reconciliation, disk-equals-saved-content detection) but
  every function reads/writes a `WorkspaceStore`'s `.getState()`/`.setState()`
  and fires toasts. This is editor-buffer domain logic more than workspace
  *scoping*, and it is Phase-4-entangled in its current form — noted here
  because it doesn't fit any of the seven named areas cleanly and is real
  logic, not because it belongs in this bucket.
- **`reset-workspace-scoped-stores.ts`, `open-file-content.ts`** — Phase 4
  store-lifecycle and fetch-orchestration glue.
- **`workspace-slot-style.ts`** — despite being "pure" and unit-tested as
  such, its output is CSS (`display:none`/`display:contents`) and a DOM
  `inert` flag for a webview mounting strategy. Out of scope for the same
  reason as `appearance-bootstrap.ts` (§4): GPUI has no DOM to hide via CSS
  display — an inactive workspace's `Entity<T>` simply isn't rendered this
  frame. Not a port target even though the code itself is trivially pure.

### gpui-free?

Yes for the genuine set: `workspace-scope.ts`, `workspace-scope-url.ts`,
`placeholder.ts`, `branch-workspace.ts`, `keep-alive-policy.ts`. No DOM, no
React, no store import. `activation-freshness.ts` is gpui-free in the same
narrow sense but is Phase-4-shaped by substance (see above).

### Already done in `crowbar-proto`/`crowbar-client`

None. `WorkspaceScope` has no generated counterpart — unsurprising, since it
is a purely frontend-side derivation from the router URL, not a daemon
response shape.

### Tests

| test file | cases | covers |
|---|---|---|
| `lib/workspace-scope.test.ts` | 7 | registry + path parsing |
| `lib/workspace/placeholder.test.ts` | 8 | placeholder detection + reason |
| `lib/workspace/branch-workspace.test.ts` | 6 | branch→workspace lookup |
| `lib/workspace/resolve-root-path.test.ts` | 4 | (store-entangled, not core) |
| `features/workspace/lib/keep-alive-policy.test.ts` | 11 | `planRetention` |
| `features/workspace/lib/activation-freshness.test.ts` | 7 | (borderline, see above) |
| `features/workspace/lib/home-workspace-resolver.test.ts` | 5 | (Phase 4, not core) |
| `features/workspace/lib/external-buffer-sync.test.ts` | 7 | (Phase 4/editor-buffer, not core) |
| `features/workspace/lib/reset-workspace-scoped-stores.test.ts` | 3 | (Phase 4, not core) |
| `features/workspace/lib/workspace-slot-style.test.ts` | 3 | (presentation, not core) |

**‼️ Finding: `workspace-scope-url.ts` (`workspaceBase`, `isHomeWorkspace`) has
no dedicated test file** — no `workspace-scope-url.test.ts` exists anywhere
under `__tests__/`. Every workspace-scoped API call in the app runs through
`workspaceBase`, making it one of the most load-bearing functions in this
whole survey, and it is untested in isolation (only indirectly, through
whatever calls it in other suites).

**Workspace-scoping Tier A test total: 7+8+6+11 = 32 cases** across 4 files
(registry/parsing, placeholder, branch-lookup, retention policy) — plus
`activation-freshness.test.ts`'s 7 cases if the borderline call above is
read the other way (giving 39).

## 7. Review threads

*(pending)*

---

## Theme tokens (also named in §16 Phase 3 Tier A, alongside `core`/`proto`/`client`)

*(pending — cross-check against §3.3's 274 measured `--` declarations and
whatever token-adjacent logic, if any, sits outside `styles/*.css`)*

---

## The headline denominator

*(pending — files / lines / test cases, split by bucket: Tier A core ·
Phase 4 state · already done (proto/client) · presentation · out of scope)*

## Findings — corrections to the brief

*(pending)*
