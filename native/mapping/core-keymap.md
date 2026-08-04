# `crowbar-core::keymap` — the keymap-resolution area (P3.70)

Five files from `native/mapping/tier-a-denominator.md` §3 ("Keymap
resolution") ported into `native/crates/crowbar-core/src/keymap/`, the third
Tier A area to land after workspace scoping (P3.53) and the git model
(P3.67). This is the area §11.2 of the design spec itself uses as its worked
example of a closed, binary item: *"`crowbar_core::keymap::resolve` passes
the ported test suite at ≥98% coverage."* Each module carries its own doc
comment citing the TS source and any surprising behaviour; this file is the
cross-module index: what each module models, what was deliberately not
ported and why, and the two non-portable `chord.ts` exports.

## 0. What's different about this item: three files had zero existing tests

Unlike the git-model area, three of the five source files
(`types.ts`, `defaults/keybinding-presets.ts`, `utils/effective-keymaps.ts`)
have **no TS test file at all** —
`tier-a-denominator.md` §3's own finding about `effective-keymaps.ts`:
*"the file that most literally IS 'keymap resolution' has zero dedicated
tests… new tests would have to be authored, not translated."* Every test in
`presets.rs` and `effective_keymaps.rs` (and the type-level assertions
embedded in both) was **authored** against the TS source's own read-through
semantics, not translated from an existing suite — each test module says so
explicitly at the top. `registry.rs` (9 cases) and `chord.rs` (5 of its 7
cases — the other 2 exercise the non-ported `eventMatchesChord`) port an
existing suite; both files also have a handful of authored "new: not
exercised by the TS suite" cases alongside the ported ones, in the style
`git/git_status_to_changed_files.rs` already established.

Three of the trickiest branches in `effective_keymaps.rs` are
mutation-tested — the implementation was actually broken three different
ways, `cargo test` actually run each time, the real failure output pasted
into the doc comment, then reverted. See §3 below for a summary and the
source file itself for the full transcripts.

`wc -l` on each Rust file (doc comments, tests and all) against the TS
source's line count from `tier-a-denominator.md`:

| module | ported from | TS lines | Rust `wc -l` | TS tests? |
|---|---|---|---|---|
| `types.rs` | `types.ts` | 52 | 115 | none — authored here |
| `registry.rs` | `registry.ts` | 220 | 361 | `registry.test.ts` (9 cases), ported |
| `presets.rs` | `defaults/keybinding-presets.ts` | 49 | 165 | none — authored here |
| `chord.rs` | `utils/chord.ts` (4 of 6 value exports) | 124 | 284 | `chord.test.ts` (7 cases; 5 for the 4 ported exports, 2 for the non-ported `eventMatchesChord`), ported |
| `effective_keymaps.rs` | `utils/effective-keymaps.ts` | 71 | 436 | none — authored here |
| `mod.rs` | (new — index + scope rationale) | — | 62 | — |

The Rust files run larger than their TS sources for the same reasons
`core-git.md` records: doc comments citing the source, ported-vs-authored
test provenance notes, and (for `effective_keymaps.rs`) three real
mutation-testing transcripts pasted verbatim.

## 1. What each module models

- **`types`** — the schema: [`Command`], [`CommandCategory`],
  [`KeymapPresetId`], [`KeymapPreset`], [`KeymapOverrides`],
  [`BindingSource`]/[`EffectiveBinding`]. `CommandCategory` and
  `KeymapPresetId` are Rust enums rather than the TS closed string unions —
  see the module doc for why this makes the TS runtime type guard
  (`isKeymapPresetId`) partially redundant, and where its behaviour still
  had to be preserved (`presets::parse_keymap_preset_id`, for the one
  untrusted-string boundary: a persisted preference).
- **`registry`** — the static command table (`COMMANDS`) plus `get_command`
  and `CATEGORY_ORDER`. `get_command` is a linear scan over a 20-entry
  `const` array rather than a `HashMap`/`LazyLock` index, matching this
  crate's stated reluctance (`crate::workspace`'s own module doc) to
  introduce global state for a table this small.
- **`presets`** — the two built-in presets (`DEFAULT_PRESET`,
  `COMPACT_PRESET`), `KEYMAP_PRESETS`, `KEYMAP_PRESET_OPTIONS`, `get_preset`,
  `parse_keymap_preset_id`. `KeymapPreset::bindings` is a
  `&'static [(&'static str, &'static str)]` slice, not a `HashMap`, so the
  whole table stays `const`-constructible with no runtime map-building step
  — the same "just scan it, it's tiny" call as `registry::get_command`.
- **`chord`** — the chord grammar: `parse_chord`, `stringify_chord`,
  `normalize_chord`, `format_chord`. See §2 for what's not here.
- **`effective_keymaps`** — the resolution algorithm itself:
  `resolve_binding` (default → preset → user precedence, all chords
  normalized), `get_effective_bindings`, `get_effective_chord_map`,
  `find_conflicting_commands`. This is the module the design spec's own
  example item names.

## 2. `chord.ts` — the two non-portable exports, and one dead one

`chord.ts` has 6 value exports (plus a type-only `ParsedChord` interface,
not counted). Of the 6:

- **Ported (4):** `parseChord`, `stringifyChord`, `normalizeChord`,
  `formatChord` — pure string-grammar functions, fully gpui-free.
- **Not ported (2): `chordFromEvent` and `eventMatchesChord`.** Both take a
  DOM `KeyboardEvent` directly and read `e.metaKey`/`e.ctrlKey`/
  `e.shiftKey`/`e.altKey`/`e.key`. The grammar they call into
  (`parseChord`/`stringifyChord`) is exactly what's ported above; the
  event-field extraction is not portable pure logic — it's a
  reimplementation-at-the-boundary, not a port. **Where that responsibility
  has to live instead:** GPUI delivers its own `KeyDownEvent`/`Modifiers`
  shape (see the gpui skill) at the window/action-dispatch layer, which is
  `crowbar-app`/`crowbar-ui` territory (Phase 3), not `crowbar-core`. The
  eventual GPUI integration has to write the equivalent of
  `chordFromEvent`/`eventMatchesChord` against `KeyDownEvent` directly,
  producing a chord string it can then hand to
  `crowbar_core::keymap::chord::normalize_chord` /
  `effective_keymaps::find_conflicting_commands` — the pure half of the
  boundary is exactly what this item ported, and it's the only half that
  can live in a `gpui`-free crate by construction.

**A seventh export, `MOD_ORDER`, is dead — found during this item, not in
the SETUP scope.** `export { MOD_ORDER }` at the bottom of `chord.ts` has
**zero non-test importers anywhere in `web/src`** (confirmed by direct
`grep -rn "MOD_ORDER" web/src`, one hit: its own declaration/export line).
`tier-a-denominator.md` §3's "4 of its 6 exports" count doesn't mention it
either way — it's simply absent from that count, the same way the survey's
own §0 methodology found `hooks/use-command-shortcut.ts` to be a live call
site around a dead-by-construction stub. `MOD_ORDER` is the inverse shape:
a dead export in an otherwise-live file. Not ported.

## 3. Mutation-tested branches in `effective_keymaps.rs`

Full transcripts live in the source file's doc comments (each `#[test]`
that was mutation-tested has one directly above it). Summary:

1. **Override-over-preset precedence.** Swapped the two `if let` checks in
   `resolve_binding` so preset was checked first. `a_user_override_wins_over_a_preset_binding_for_the_same_command`
   failed (`left: "mod+]"` — the preset's answer — `right: "mod+z"` — the
   override that should have won); 12 other tests still passed. Reverted.
2. **Self-exclusion in `find_conflicting_commands`.** Removed the
   `binding.command_id != command_id` clause. **Two** tests failed, not
   one — recorded honestly rather than trimmed to fit a tidier story:
   `find_conflicting_commands_excludes_the_queried_command_even_when_its_own_chord_matches`
   (the test purpose-built to catch this) and, incidentally,
   `two_commands_resolving_to_the_same_chord_conflict_with_each_other`
   (whose query command's id happens to also share the queried chord).
   Both are real evidence the guard does something. Reverted.
3. **The empty-string-override edge case.** Changed the override lookup to
   filter out empty-string values (treating `""` as "no override" — the
   plausible-but-wrong reading). `an_empty_string_override_still_wins_as_user_source`
   failed (`left: "mod+a"` — fell through to default — `right: ""` — the
   correct "explicitly unbound" answer); every other test still passed.
   Reverted.

This last one is also a real behavioural finding worth stating plainly: TS
`resolveBinding`'s `if (userChord != null)` check means an override whose
*value* is the empty string still takes the `'user'` branch (an empty
string is not `null`/`undefined`), producing `chord: '', source: 'user'` —
a real, distinct "the user explicitly unbound this command" outcome, not an
edge case that collapses into "no override." `resolve_binding` reproduces
this exactly; see its module doc for the full argument.

## 4. A correction to `tier-a-denominator.md` §3: the registry has 20 commands, not 19

The survey calls `registry.ts` *"the static 19-command table"* in both its
file-table row and its own prose. Counting `id:` occurrences directly in the
TS source (`awk '/^export const COMMANDS/,/^\]/' registry.ts | grep -c
'^\s*id:'`) gives **20**. `registry.rs`'s `COMMANDS` array, `CATEGORY_ORDER`
coverage test, and every count-dependent test in this item use the real
figure (20) — confirmed against the compiler, which rejects an array-length
mismatch outright (this is exactly how the discrepancy was first caught:
`cargo build` refused to compile a `[Command; 19]` binding for a 20-element
array literal).

## 5. What was not ported, and why

- **`stores/store.ts`** (100 lines) — the zustand store persisting the
  active preset and user overrides to `localStorage`
  (`crowbar:settings:keybindingPreset`/`…UserOverrides`). Phase 4
  (`Entity<T>`) + D6 (the persistence mechanism itself is deleted, replaced
  by the daemon's `/v0/settings/ui`, which stores opaque JSON — the schema
  this store would validate against is exactly [`types::KeymapOverrides`]
  above, already ported). Nothing in this item constructs or persists one.
- **The four keyboard hooks** (`use-effective-keymap`, `use-save-keyboard`,
  `use-sidebar-tab-keyboard`, `use-workspace-switcher-keyboard`) — `useEffect`
  + DOM `addEventListener('keydown', …)` wiring that calls into
  `getEffectiveChordMap`/dispatches on a resolved chord. Not logic: GPUI has
  its own native action/keybinding dispatch system (see the gpui skill), so
  this glue is replaced wholesale at Phase 3, not translated.
- **`hooks/use-command-shortcut.ts`** — already dead upstream (a 4-line stub
  that unconditionally `return undefined`s), per `tier-a-denominator.md`
  §3's own verdict. Not ported for the same reason `MOD_ORDER` isn't (§2).
- **`chord.ts`'s `chordFromEvent`/`eventMatchesChord`** and **`MOD_ORDER`** —
  see §2.

## 6. Coverage

`cargo llvm-cov -p crowbar-core`: **100.00% line coverage over 1882 lines**
(up from 1435 before this item — the P3.67 git-model baseline). This item
added 447 covered lines, all at 100.00%: `types.rs` 7, `registry.rs` 53,
`presets.rs` 47, `chord.rs` 141, `effective_keymaps.rs` 199 (executable
lines as `cargo llvm-cov` counts them — smaller than each file's `wc -l`
from §0, which also counts doc comments and blank lines).

`chord.rs` is the one file with a *region* gap (96.62%, 7 missed regions)
while still reporting 100.00% *lines* — the same shape `workspace/scope.rs`
already has in this crate (99.72% regions, 100.00% lines), not a new kind of
gap this item introduced.

171 tests in `crowbar-core`'s lib target (up from 130 before this item: 41
new `keymap::*` test cases across the five modules — 11 for `registry` (9
ported + 2 authored), 10 for `chord` (5 ported — the TS suite's other 2
exercise the non-ported `eventMatchesChord` — + 5 authored), 7 authored for
`presets`, and 13 authored for `effective_keymaps` (0 for `types`, which has
no functions of its own to test); `presets`/`effective_keymaps` had no TS
suite to port from at all. See each module's own `#[cfg(test)]` block for
the exact ported-vs-authored split.

Full-workspace gates, run in the foreground: `cargo clippy --workspace
--all-targets -- -D warnings` clean; `cargo test --workspace` — 2271 passed,
0 failed (up from a 2230-test trunk baseline plus this item's 41 new
`crowbar-core` tests); `bash native/scripts/check-invariants.sh` — 7 of 7
invariants pass.
