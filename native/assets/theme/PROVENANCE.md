# Vendored theme CSS — the token source of truth

These are the **actual CSS files** the React app ships, copied verbatim. They
are the input to `crates/crowbar-ui/tools/gen-theme.py`, which resolves them
(`var()` chains, `oklch()`, `color-mix()`, `calc()`) into `src/theme/generated.rs`.

## Why they live here and not in `web/`

The generator used to read `web/src/styles/theme.css` directly. That was
correct while `web/` existed, and worthless the day it is deleted: the tokens
would survive only as Rust literals whose derivation nobody could re-check,
which is the "extrapolated into Rust values" failure mode this vendoring
exists to prevent.

Vendored here, the CSS **is** the source of truth, permanently, exactly as
`assets/fonts/` and `assets/icons/` are for typefaces and artwork. `native/`
depends on nothing outside itself, and the tokens remain re-derivable from the
same bytes the React app used.

| file | upstream (while it exists) |
|---|---|
| `theme.css` | `web/src/styles/theme.css` |
| `file-explorer-tree.css` | `web/src/features/file-explorer/styles/file-explorer-tree.css` |

## Keeping them honest

Two checks, both independent of each other:

* `gen-theme.py --check` regenerates `generated.rs` and fails if it differs, so
  the Rust table cannot drift from the CSS.
* `verify-theme.py` builds a page from **these bytes**, asks Chrome for every
  token's computed value in both appearances, paints each to a canvas, and
  diffs the 8-bit sRGB against the constants in `generated.rs`. It checks the
  generator against a real browser rather than against another transcription.

While `web/` still exists, `gen-theme.py --check-vendored` also diffs these
files against their upstreams, so a change there is a loud failure here rather
than a silent divergence.
