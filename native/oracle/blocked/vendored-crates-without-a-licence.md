# Blocked — two vendored crates we ship declare no licence at all

**Found:** 2026-07-30, Phase 0, while writing `NOTICE.md` · **Not a blocker.**
**Status:** needs a human to confirm with Zed, or a decision to accept the risk.

## The facts, verified against upstream

`gpui_shared_string` and `gpui_util` are vendored under `native/vendor/zed-deps/`
and **both are compiled into the macOS binary**. Neither declares a licence.

I checked upstream directly at our pinned SHA
(`zed-industries/zed` @ `1a246efd`), not just our copy:

| | `license` key |
|---|---|
| `crates/gpui_shared_string/Cargo.toml` | **absent** — only `name`, `version`, `publish.workspace`, `edition.workspace` |
| `crates/gpui_util/Cargo.toml` | **absent** — same shape |
| Zed root `[workspace.package]` | **absent** — it declares only `publish` and `edition` |

So there is nothing to inherit, and **our de-inheritance dropped nothing** — this
had to be ruled out, because the vendoring step rewrote every
`something.workspace = true` into a concrete value, and a dropped `license` key
would have looked identical to an absent one. It is genuinely absent upstream.

Each crate directory carries a `LICENSE-APACHE` file. **That is not
authoritative**, and this tree contains the proof: `zed-deps/path/` ships
*only* `LICENSE-APACHE`, under a Zed copyright line, while its manifest declares
`license = "GPL-3.0-or-later"`. An audit that reads licence files instead of
manifest keys gets `path` exactly backwards — and the §10.6 audit did.

`native/vendor/PINNED.md` asserts these two "fall back to Zed's repo-level dual
Apache-2.0 / GPL-3.0 licensing." That claim could not be verified from anything
in-tree, and the root manifest above does not support it.

## Why it does not block

Crowbar is **AGPL-3.0-only** (D1). Both plausible answers are fine:

- if Apache-2.0 → compatible, no obligation beyond attribution;
- if GPL-3.0 → AGPLv3 §13 expressly permits the combination, which is the whole
  reason D1 was taken.

There is no realistic third answer: they are Zed's own first-party crates in a
repo that dual-licenses Apache-2.0/GPL-3.0.

So this is an **attribution-accuracy** question, not a legal-exposure one.
`NOTICE.md` records them in a separate "no declaration" subsection rather than
silently listing them as Apache-2.0, which is the honest position until someone
resolves it.

## What would resolve it

Any one of:

1. Ask Zed (an issue or PR adding the `license` key upstream — this is a real
   upstream gap and worth reporting regardless).
2. Read Zed's root `LICENSE-APACHE` / `LICENSE-GPL` and any `COPYING`/README
   statement about crates that omit the key, and decide it covers them.
3. Decide the distinction does not matter to us, given both candidates are
   compatible, and note that decision in `NOTICE.md`.

## Related, and worth knowing

`path` is GPL-3.0-or-later and is **not** currently linked — it is reached only
via `http_client → util → path`, and `http_client` declares `util` optional
behind feature `github-download`, which is **defined but never enabled** in
`native/`. Confirmed absent from
`cargo tree -e normal,build --target aarch64-apple-darwin`.

**One feature flip puts a GPL crate into the binary.** That is fine under D1,
but `NOTICE.md`'s linked/not-linked column would silently go stale. If
`github-download` is ever enabled, update it.
