# `crowbar-core::color` — CSS colour parsing (theme-token colour math)

`resolve-css-color.ts`'s pure region ported into
`native/crates/crowbar-core/src/color.rs`, extending — not replacing — the
module that already carried `color-mix(in srgb, …)` (`Srgba`, `color_mix`,
`color_mix_remainder`). This is the last unported area named in
`native/mapping/tier-a-denominator.md`'s "Theme tokens" section (one of the
four remaining Tier A areas per the item brief).

## 0. Why this extends `color.rs` instead of landing as a new module

The item brief's default instruction is "port into a new module." This item
deviates from that, deliberately: `tier-a-denominator.md`'s own theme-tokens
section says the ported tests "would extend `crowbar-core/src/color.rs`'s
existing 13 `#[test]`s rather than start a new file," and `color.rs`'s own
pre-existing module doc already states the split — `color-mix` arithmetic
lives there "because it is arithmetic on four floats: no window, no
framework, no `gpui`," with `crowbar-ui` converting at the boundary. CSS
colour *parsing* (`#hex`/`rgb()`/`oklch()` → a colour) is the same kind of
thing solving the other half of the same problem (evaluating a `theme.css`
token needs both parsing *and* mixing), so it belongs beside `color_mix` in
the same file rather than in a sibling one. `color.rs` is already registered
in `lib.rs` as `pub mod color;`, so no new module registration was needed.

## 1. Scope: what's pure, what's DOM, and the survey's own line estimate

`tier-a-denominator.md` describes `resolve-css-color.ts` (187 lines, `wc -l`)
as *mixed*: a pure-logic half and a DOM-entangled half in the same file, and
says so explicitly — its own §1 flags this file as one of the seven where
"the line count given is an estimate of the pure-logic region only." Its
theme-tokens table gives that estimate as **~130 lines**.

Measuring the actual boundary directly (`resolve-css-color.ts`'s
`cssColorToHex` closes at line 85; `SYNTAX_TOKEN_KEYS`, the first
DOM-adjacent declaration, starts at line 87) gives **85 lines** for the pure
region — the module's own header comment (5 lines, describing the file as a
whole, not just the pure half) plus seven functions: `clamp255`,
`toHexByte`, `expandShortHex`, `gammaEncode`, `oklchToHex`, `parseAlpha`,
`cssColorToHex`. This is a correction to the survey's own estimate in the
same spirit as `core-keymap.md` §4's registry-count correction: **85, not
~130**, `wc -l`-exact on the pure region's real boundary rather than an
estimate. The other 102 lines (87–187: `resolveCssVar`, `readSyntaxPalette`,
`readTerminalPalette`, `SYNTAX_TOKEN_KEYS`, `TERMINAL_ANSI_KEYS`, and their
two exported key-list types) are the DOM-entangled half — see §3.

Only `cssColorToHex` carries an `export` keyword among the seven; the other
six are module-private helpers reachable only through it — the same
"exported entry point over private helpers" shape §8 of the survey found in
`review-code-view.tsx`'s embedded region. `resolve-css-color.test.ts`'s own
`describe('cssColorToHex', …)` block (8 cases, all through the one exported
function) confirms this is the whole reachable pure surface — nothing here
was left un-exercised because it was unreachable.

## 2. Liveness

**TS side (why this was worth porting at all):** `cssColorToHex` is LIVE.
`resolveCssVar` calls it directly; `resolveCssVar` is called by
`readSyntaxPalette`/`readTerminalPalette`; those are called by
`use-terminal-theme.ts`, imported by the terminal, which
`tier-a-denominator.md` establishes is unconditionally mounted
(`TerminalHost`, `ide-shell.tsx`). Confirmed independently for this item:
`grep -rn "cssColorToHex\|resolve-css-color" web/src` (excluding the test
file) shows only `use-terminal-theme.ts`, `mermaid-theme.ts`, and
`monaco/define-theme.ts` importing from the module — all three real,
non-test call sites, consistent with the survey's LIVE verdict.

**Rust side:** as of this item, nothing in the `native/` workspace calls
`parse_css_color` yet — grep confirms zero non-test callers. This is not a
liveness problem, and it is not a new shape for this port: `crowbar-core` is
being built ahead of the GPUI wiring that will consume it (`core-keymap.md`
§2 records the identical state for the whole `keymap` module — none of it
is wired into an action-dispatch system yet either). The liveness gate that
matters at this stage is the *TS-side* one above: is the source code being
ported dead in the app it came from. It is not.

A closely related, worth-recording finding: `crowbar-ui/tools/gen-theme.py`
— the Python script that bakes `theme.css`'s `oklch()`/`rgb()` values into
`crowbar-ui/src/theme/token.rs`'s `Color` constants at generation time —
**already has its own, independent `oklch_to_srgb`**, with the identical
OKLab matrices, checked against Tailwind's own published hex values via its
`check_palette()`. That script does not import from or depend on
`crowbar-core` at all; it is a separate, offline, Python-side tool. So this
item's `oklch_to_srgb` does not currently close a build-blocking gap the way
`tier-a-denominator.md`'s "concrete, actionable gap" framing might suggest —
the real generation pipeline already solves its own version of this problem
in a different language. What this item does provide is the **Rust-side**
equivalent, gpui-free and tested, available to any future Rust caller (a
settings-driven custom-theme feature parsing a user-supplied CSS colour
string, for instance) without going through Python. §6 below reuses one of
`gen-theme.py`'s own fixtures as a cross-check between the two independent
implementations.

## 3. What was not ported, and why

- **`resolveCssVar`** — reads a CSS custom property off the live DOM via
  `getComputedStyle()`, with a temporary-element `var()`-resolution trick
  for indirect references. The file's own comment says why it exists at
  all: "Monaco and xterm cannot read CSS variables." This is a webview
  artifact with no native counterpart — `crowbar-terminal`/`crowbar-editor`
  read `theme.foreground`/`theme.syntax.keyword` directly off the sealed
  `Theme` struct, never off a computed style.
- **`readSyntaxPalette`/`readTerminalPalette`/`readPalette`** — thin loops
  over `resolveCssVar`; DOM-entangled by the same transitive reasoning.
- **`SYNTAX_TOKEN_KEYS`/`TERMINAL_ANSI_KEYS`** (and their `SyntaxTokenKey`/
  `TerminalAnsiKey` types) — the key lists these two palette readers loop
  over. Pure data, but consumed only by the DOM-entangled half; not ported
  for the same reason the readers that use them were not. (The equivalent
  Rust-side schema, when `crowbar-ui`'s `Theme`/token generation needs one,
  is a `crowbar-ui` concern per §4.3 rule 3 — token *shape*, not `core`'s
  arithmetic.)
- **`toHexByte`/`clamp255`** — not carried over as named functions. Both
  exist in the source purely to quantise a float into an 8-bit hex digit
  pair; [`Srgba::new`]'s existing `0.0..=1.0` clamp is this crate's
  equivalent of `clamp255`, and there is nothing here that needs
  `toHexByte`'s hex-*string* formatting at all — see §5.

## 4. Line counts (both units, `wc -l`)

| | TS lines | Rust lines |
|---|---|---|
| Pure region ported (`resolve-css-color.ts` lines 1–85) | **85** | — |
| `color.rs` net growth (`git diff --stat`: +367/−1) | — | **366** |
| … of which: module-doc extension | — | ~11 |
| … of which: `parse_css_color` + 6 private helpers (incl. doc comments, banner) | — | ~192 |
| … of which: tests (8 ported + 2 authored, incl. banners) | — | ~162 |

Both units stated explicitly, per this survey's own standing correction
(`tier-a-denominator.md`'s note on a past report that divided Rust lines by
TypeScript lines and got 116%): **85 TS lines of pure logic ported; 366 net
new Rust lines**, the large ratio explained the same way `core-keymap.md`
§0 explains it for `keymap` — doc comments citing the TS source line-for-
line, ported-vs-authored test provenance notes, and (here) the extra
cross-check against `gen-theme.py`'s independent implementation.

## 5. A design decision: `Srgba`, not a hex string

`cssColorToHex` returns `string | null` — a `#rrggbb[aa]` string, because
that is what Monaco/xterm's DOM APIs consume. `parse_css_color` returns
`Option<Srgba>` instead. There is no hex-string consumer in `crowbar-core`
or anywhere reachable from it; `crowbar-ui` converts at its boundary the
same way it already does for `color_mix_remainder` (see `token.rs`'s
`Color::to_srgba`/`from_srgba`). Returning `Srgba` directly is strictly more
precise than the source, not merely different: the TS pipeline round-trips
every channel through an 8-bit hex byte (`toHexByte(alpha * 255)` etc.), so
`rgba(0, 0, 0, 0.5)` comes back out of `cssColorToHex` as `#00000080` —
alpha 128/255 ≈ 0.50196, not 0.5. `parse_css_color("rgba(0, 0, 0, 0.5)")`
returns alpha `0.5` exactly. The ported tests assert the exact value, not
the source's lossy one — this is stated directly in both the function's own
doc comment and inline in `converts_rgb_and_rgba`'s test.

A second, smaller fidelity decision: every numeric token in `rgb()`/`oklch()`
is validated against `[\d.]+` (`parse_decimal`) before parsing — the exact
character class the TS regex used — rather than accepting anything Rust's
`f32::from_str` does (which would also accept `-1` or `1e2`, neither of
which the source's anchored regex ever matched). This keeps the port exactly
as strict as the source rather than quietly more lenient.

## 6. Tests: 8 ported, 2 authored

`resolve-css-color.test.ts` has 13 cases across two `describe` blocks: 8 in
`describe('cssColorToHex', …)` (the pure half, ported here 1:1 — see each
test's doc comment naming its TS source) and 5 in `describe('DOM resolver',
…)` (the DOM half, not ported, §3). **8/8 of the portable cases are
ported**, matching `tier-a-denominator.md`'s own count exactly.

Two more are authored, not translated from the TS suite:

- `rejects_malformed_input_in_every_recognised_prefix` — six ways to fail
  that the TS regex's single anchored match collapses into one rejection
  (wrong hex length, non-hex digits, a trailing token past `rgb()`'s alpha,
  a missing `oklch()` component, an unparseable `oklch()` alpha, a trailing
  token past `oklch()`'s alpha), but that this hand-written parser reaches
  as separate branches worth confirming individually.
- `cross_checks_a_tailwind_swatch_against_gen_theme_pys_fixture` — reuses
  one fixture from `gen-theme.py`'s own `TAILWIND` table (Tailwind's
  published `red-500`: `oklch(63.7% 0.237 25.331)` → `#fb2c36`) as an
  independent check against a wholly separate implementation of the same
  reference algorithm (§2).

## 7. Mutation testing, actually performed

Two mutations were made, run, confirmed red, and reverted (both diffs
inspected before and after; `git diff` was clean afterward):

1. **`oklch_to_srgb`'s red-channel coefficient**, `4.076_741_7` →
   `4.176_741_7` (a plausible transcription slip in a hand-copied magic
   constant). Result: 22 of 23 tests in `color::tests` still passed —
   including both `converts_oklch_endpoints_exactly` (saturates to `1.0`
   either way once clamped) and `converts_a_known_chromatic_oklch_within_
   tolerance_of_srgb_red` (the mutated value is still `> 250/255`, so the
   TS-ported test's own loose tolerance does not catch it either). Only
   `cross_checks_a_tailwind_swatch_against_gen_theme_pys_fixture` failed:
   `r: got 1 (~255), want 0.9843137 (~251) — off by more than 1/255`. This
   is a genuine, honest finding, not a tidied-up story: the 8 ported TS
   tests alone would not have caught this specific mutation, because two of
   them saturate or tolerate it — the authored cross-check test is the one
   that actually exercises the coefficient's exact value against an
   independently-sourced answer. Reverted; `cargo test -p crowbar-core
   color::` back to 23/23.
2. **The trailing-token guard in `parse_rgb_functional`** (`if
   tokens.next().is_some() { return None }`) — removed entirely, since it
   has no TS-regex equivalent and is exactly the kind of hand-written guard
   this project's own history has previously shipped vacuous (`QUEUE.md`'s
   "Vacuous Guard Tests" finding: eight guards tested the declaration, not
   the behaviour). Result:
   `rejects_malformed_input_in_every_recognised_prefix` failed immediately —
   `left: Some(Srgba { r: 1.0, g: 0.0, b: 0.0, a: 0.5 })`, `right: None`,
   labelled `"trailing token past rgb()'s alpha"`. The guard is real, not
   vacuous. Reverted; `cargo test -p crowbar-core color::` back to 23/23.

## 8. Gates

Run from `native/`, `PATH` including `$HOME/.cargo/bin`:

- `cargo build --workspace` — clean (only the standing `block v0.1.6`
  future-incompatibility note, not ours).
- `cargo clippy --workspace --all-targets -- -D warnings` — clean.
- `cargo test --workspace` — **2534 passed, 0 failed** (baseline 2524 + 10
  new `color::tests` cases: 8 ported + 2 authored).
- `./scripts/check-invariants.sh` — **7 of 7 invariants pass**, including
  rule 4 (no raw colour construction outside `crowbar-ui/src/theme/` —
  `Srgba::new`/`Srgba { … }` in `color.rs` itself does not match the rule's
  `Hsla {`/`Rgba {`/`rgba?\(`/`hsla?\(` patterns, by the same
  non-identifier-boundary reasoning the rule's own comment documents for
  `Srgba`) and rule 5 (`cargo fmt --check`), which needed one `cargo fmt -p
  crowbar-core` — safe to run un-scoped-by-hand because `git status` showed
  only this item's own file, `color.rs`, dirty at the time.
