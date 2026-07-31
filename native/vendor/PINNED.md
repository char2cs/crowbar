# Vendored `gpui` + `gpui-component` — the pin record

Spec §10.5: *"`gpui` is not a released crate. Vendor it at `native/vendor/gpui/`,
pinned to a SHA. Zed's churn then reaches us only when we choose to take it."*

Everything under `native/vendor/` is third-party source, vendored verbatim except
for the mechanical manifest edits listed under [Deviations](#deviations-from-upstream).
Do not hand-edit it; re-run the vendoring steps in [How to re-pin](#how-to-re-pin).

**The short version.** Two configurations were evaluated end to end:

| | result |
|---|---|
| **crates.io** — `gpui = "0.2.2"` + the published `gpui_*` support crates | ❌ `gpui-component` @ `88f102d1` fails with **338 errors across 75 files**. Nine months of API drift. |
| **vendored subtree** — this directory, de-inherited manifests, `path` deps | ✅ builds; `--locked` release binary on `aarch64-apple-darwin`. |

The crates.io route is genuinely the better shape if it ever works, so it is
re-checked at every re-pin (step 8). It does not work today.

The commonly-cited cost of the git route — being forced to adopt Zed's
`[patch.crates-io]` fork set, livekit and all — **does not apply to a
consumer** and was verified not to have happened here: see
[Zed's `[patch.crates-io]` costs us nothing](#zeds-patchcrates-io-costs-us-nothing--verified).

---

## The pins

| what | pin | date |
|---|---|---|
| `gpui` (and every in-repo sibling) | `zed-industries/zed` @ `1a246efd7e1b83ab568ec5e3e6c1a43a42e1abba` | 2026-07-15 |
| `gpui-component` / `-macros` / `-assets` | `longbridge/gpui-component` @ `88f102d13654fe25aa2fede076274b6b751a3704` | 2026-07-30 |

`gpui-component-assets` is **not** a separate repository — it is
`crates/assets` in the same `gpui-component` tree, so it shares the SHA above.
Crate versions at that SHA: `gpui-component` 0.5.2, `gpui-component-macros` 0.5.1,
`gpui-component-assets` 0.5.1, `gpui` 0.2.2.

`88f102d1` was the tip of `longbridge/gpui-component`'s default branch (`main`)
when this was vendored. Its `Cargo.lock` resolves every `zed-industries/zed` git
dependency to `1a246efd…`, so that is the `gpui` revision the pair was tested
against upstream — it was **not** chosen independently.

### Why not crates.io — measured, not assumed

**Correction to spec §10.5: `gpui` *is* a released crate.** `gpui` 0.2.2 is on
crates.io (published 2025-10-22, ~178k downloads) and it is self-contained: the
support crates ship alongside it under renamed packages — `gpui_collections`,
`gpui-macros`, `gpui_http_client`, `gpui_media`, `gpui_refineable`,
`gpui_sum_tree`, `gpui_util`, `gpui_util_macros`, all at ^0.2.2. So
`gpui = "0.2.2"` is a real, available, and in principle *stronger* pin than a
vendored subtree, with far less machinery. It was evaluated properly rather
than dismissed.

**It does not work.** `gpui-component` @ `88f102d1` compiled against crates.io
`gpui` 0.2.2 fails with **338 errors across 75 of its source files**:

```
$ cargo check -p gpui-component     # gpui = "0.2.2", gpui_macros = { version = "0.2.2", package = "gpui-macros" }
error: could not compile `gpui-component` (lib) due to 338 previous errors; 6 warnings emitted
```

| errors | code | class |
|---:|---|---|
| 139 | E0599 | method not found — `Pixels::as_f32` alone accounts for **86** call sites; `role()` (the accessibility API) 29 more; then `flex_grow_1`, `inset`, `top_center`, `min_size_full`, `container_query`, `on_aux_click`, `is_middle_click` |
| 56 | E0308 | mismatched types |
| 54 | E0061 | wrong arity — 41 of them "takes 1 argument but 2 were supplied"; signatures moved |
| 39 | E0277 | unsatisfied trait bounds, mostly `ElementId` and `SliderState` |
| 33 | E0432/E0433 | unresolved imports from `gpui` — `Anchor` (12), `Role`/`Role::*` (16) |
| 13 | E0560/E0609/E0026/E0521 | struct fields renamed or removed; two lifetime regressions |

This is nine months of API drift, not a handful of shims: the whole platform
layer has since been split out of `gpui` into `gpui_platform` / `gpui_macos` /
`gpui_linux` / `gpui_windows` / `gpui_web`, so `gpui_platform::application()` —
the entry point every `gpui-component` example uses — does not exist at 0.2.2 at
all. The crates.io release also still carries `macos-blade` and pulls
`blade-graphics` + `cosmic-text` unconditionally; the git tree renders through
Metal in `gpui_macos`.

`gpui-component` 0.5.2 on crates.io (published 2026-02-05, ~6 months behind
`main`) is the other half of the same trap — taking it would pin us to a
correspondingly older `gpui` and forfeit half a year of the widget set we chose
this library for.

So the spec's *conclusion* holds even though its premise is wrong: the usable
`gpui` is the git one. Re-test this at every re-pin — if upstream ever
publishes a `gpui` release contemporaneous with a `gpui-component` release,
crates.io becomes strictly better than this directory and it should be deleted.

---

## Zed's own toolchain at this SHA

`zed-industries/zed@1a246efd…/rust-toolchain.toml`, verbatim:

```toml
[toolchain]
channel = "1.95.0"
profile = "minimal"
components = [ "rustfmt", "clippy", "rust-analyzer", "rust-src" ]
targets = [
    "wasm32-wasip2", # extensions
    "wasm32-unknown-unknown", # gpui on the web
    "x86_64-unknown-linux-musl", # remote server
]
```

**No conflict with our intended 1.96.0 pin.** Zed pins 1.95.0 as a *floor* for
its own CI; the vendored crates are edition 2024 and build clean on stable
1.96.0 (that is the toolchain that produced the build output below). There is
deliberately **no `rust-toolchain.toml` inside `native/vendor/`** — vendoring
Zed's would silently force rustup to download 1.95.0 for the whole `native/`
tree. If a future re-pin raises Zed's floor above our pin, that is the moment to
revisit.

---

## Vendored crates

29 third-party crates (26 from Zed, 3 from gpui-component) plus the throwaway
probe.

| crate | version | licence | path under `native/vendor/` | `.rs` LOC | bytes |
|---|---|---|---|---:|---:|
| `gpui` | 0.2.2 | Apache-2.0 | `gpui` | 72 985 | 7 215 459 |
| `gpui-component` | 0.5.2 | Apache-2.0 | `gpui-component` | 91 789 | 3 233 818 |
| `gpui-component-assets` | 0.5.1 | Apache-2.0 | `gpui-component-assets` | 212 | 67 106 |
| `gpui-component-macros` | 0.5.1 | Apache-2.0 | `gpui-component-macros` | 441 | 28 923 |
| `collections` | 0.1.0 | Apache-2.0 | `zed-deps/collections` | 418 | 23 641 |
| `derive_refineable` | 0.1.0 | Apache-2.0 | `zed-deps/derive_refineable` | 548 | 28 998 |
| `gpui_linux` | 0.1.0 | Apache-2.0 | `zed-deps/gpui_linux` | 14 516 | 536 505 |
| `gpui_macos` | 0.1.0 | Apache-2.0 | `zed-deps/gpui_macos` | 12 116 | 501 681 |
| `gpui_macros` | 0.1.0 | Apache-2.0 | `zed-deps/gpui_macros` | 3 470 | 136 512 |
| `gpui_platform` | 0.1.0 | Apache-2.0 | `zed-deps/gpui_platform` | 186 | 18 131 |
| `gpui_shared_string` | 0.1.0 | *(none declared)* | `zed-deps/gpui_shared_string` | 203 | 15 903 |
| `gpui_util` | 0.1.0 | *(none declared)* | `zed-deps/gpui_util` | 721 | 31 580 |
| `gpui_web` | 0.1.0 | Apache-2.0 | `zed-deps/gpui_web` | 2 588 | 100 912 |
| `gpui_wgpu` | 0.1.0 | Apache-2.0 | `zed-deps/gpui_wgpu` | 4 081 | 212 422 |
| `gpui_windows` | 0.1.0 | Apache-2.0 | `zed-deps/gpui_windows` | 11 759 | 467 655 |
| `http_client` | 0.1.0 | Apache-2.0 | `zed-deps/http_client` | 1 309 | 52 246 |
| `http_client_tls` | 0.1.0 | Apache-2.0 | `zed-deps/http_client_tls` | 21 | 12 085 |
| `media` | 0.1.0 | Apache-2.0 | `zed-deps/media` | 406 | 25 905 |
| `path` | 0.1.0 | **GPL-3.0-or-later** | `zed-deps/path` | 1 372 | 52 601 |
| `perf` | 0.1.0 | Apache-2.0 | `zed-deps/perf` | 1 039 | 51 400 |
| `refineable` | 0.1.0 | Apache-2.0 | `zed-deps/refineable` | 132 | 16 848 |
| `reqwest_client` | 0.1.0 | Apache-2.0 | `zed-deps/reqwest_client` | 416 | 27 433 |
| `scheduler` | 0.1.0 | Apache-2.0 | `zed-deps/scheduler` | 2 760 | 101 165 |
| `sum_tree` | 0.1.0 | Apache-2.0 | `zed-deps/sum_tree` | 3 327 | 120 504 |
| `util` | 0.1.0 | Apache-2.0 | `zed-deps/util` | 9 617 | 343 732 |
| `util_macros` | 0.1.0 | Apache-2.0 | `zed-deps/util_macros` | 286 | 21 615 |
| `zlog` | 0.1.0 | **GPL-3.0-or-later** | `zed-deps/zlog` | 1 709 | 92 014 |
| `ztracing` | 0.1.0 | **GPL-3.0-or-later** | `zed-deps/ztracing` | 109 | 48 829 |
| `ztracing_macro` | 0.1.0 | **GPL-3.0-or-later** | `zed-deps/ztracing_macro` | 7 | 45 436 |
| *(probe — ours, throwaway)* | 0.0.0 | — | `probe` | 60 | 2 567 |

**Totals: 29 vendored crates, 740 files, 238 543 lines of Rust, 13 631 059 bytes
= 13.0 MiB on disk** (the probe's own 2 files / 60 lines excluded).

4.9 MiB of that single total is `gpui/examples/` — 22 demo binaries and their
PNG/SVG assets, declared as explicit `[[example]]` targets in gpui's manifest.
They are never built through a `path` dependency. They were kept rather than
deleted precisely because the manifest declares them: removing the directory
without also editing the `[[example]]` blocks would make the manifest
unloadable, and editing them is a bigger deviation than carrying 4.9 MiB.

The Zed closure was **measured**, not assumed: it is the transitive closure of
in-repo path dependencies from `gpui`, `gpui_platform` and `gpui_macros`
(normal + build deps = 24 crates; adding dev-deps pulls in `reqwest_client` and
`http_client_tls`, which are vendored too so the manifests stay loadable).
`gpui_linux`, `gpui_windows` and `gpui_web` are in the closure even though we
only build macOS: Cargo resolves target-conditional dependencies for *all*
targets, so their manifests must exist.

**21 of the 29 actually compile** in a macOS release build: `gpui`,
`gpui-component`, `-assets`, `-macros`, `collections`, `derive_refineable`,
`gpui_macos`, `gpui_macros`, `gpui_platform`, `gpui_shared_string`,
`gpui_util`, `http_client`, `media`, `perf`, `refineable`, `scheduler`,
`sum_tree`, `util_macros`, `zlog`, `ztracing`, `ztracing_macro`. The other
eight (`gpui_linux`, `gpui_windows`, `gpui_web`, `gpui_wgpu`, `http_client_tls`,
`path`, `reqwest_client`, `util`) are carried for resolution, other targets, or
dev-deps only.

---

## Licence provenance

Context: [zed-industries/zed#55470](https://github.com/zed-industries/zed/issues/55470)
alleges `gpui → sum_tree → ztracing → zlog` drags GPL into a nominally
Apache-2.0 crate. **The chain does exist at this SHA.** Four vendored crates
declare `license = "GPL-3.0-or-later"` in their own `Cargo.toml`:

| crate | licence | reached from | linked into the macOS binary? |
|---|---|---|---|
| `zlog` | GPL-3.0-or-later | `gpui → sum_tree → ztracing → zlog` | **yes** |
| `ztracing` | GPL-3.0-or-later | `gpui → sum_tree → ztracing` | **yes** |
| `ztracing_macro` | GPL-3.0-or-later | `ztracing → ztracing_macro` | **yes** (proc-macro) |
| `path` | GPL-3.0-or-later | `http_client → util → path` | no |

The first three are unconditional, non-optional dependencies of `gpui`, and all
three were genuinely compiled in the release build below — this is not a
paper finding.

The fourth is a second, independent GPL edge the upstream issue does not
mention, and it behaves differently: `http_client` declares `util` as
**optional**, nothing in our graph turns that feature on, so `util` and `path`
are vendored (the manifests must exist) but never compiled. Do not rely on that
staying true — one feature flag flips it.

Every other vendored Zed crate is Apache-2.0, except `gpui_util` and
`gpui_shared_string`, which declare no `license` field at all and so fall back
to Zed's repo-level dual Apache-2.0 / GPL-3.0 licensing.

Crowbar is relicensing to **AGPL-3.0-only**, under which AGPLv3 §13 expressly
permits combining with GPLv3 work. So this is **housekeeping, not a blocker**:
the obligation is attribution and source availability, both of which AGPL
already imposes on us. `NOTICE.md` at the repo root will need these four crates
listed under GPL-3.0-or-later.

Verbatim, run from `native/vendor/`:

```
$ cargo tree -i zlog
zlog v0.1.0 (native/vendor/zed-deps/zlog)
└── ztracing v0.1.0 (native/vendor/zed-deps/ztracing)
    └── sum_tree v0.1.0 (native/vendor/zed-deps/sum_tree)
        └── gpui v0.2.2 (native/vendor/gpui)
            ├── gpui-component v0.5.2 (native/vendor/gpui-component)
            │   └── gpui-vendor-probe v0.0.0 (native/vendor/probe)
            ├── gpui-component-assets v0.5.1 (native/vendor/gpui-component-assets)
            │   └── gpui-component v0.5.2 (native/vendor/gpui-component) (*)
            ├── gpui-vendor-probe v0.0.0 (native/vendor/probe)
            ├── gpui_macos v0.1.0 (native/vendor/zed-deps/gpui_macos)
            │   └── gpui_platform v0.1.0 (native/vendor/zed-deps/gpui_platform)
            │       └── gpui-vendor-probe v0.0.0 (native/vendor/probe)
            │       [dev-dependencies]
            │       └── gpui v0.2.2 (native/vendor/gpui) (*)
            ├── gpui_platform v0.1.0 (native/vendor/zed-deps/gpui_platform) (*)
            ├── gpui_wgpu v0.1.0 (native/vendor/zed-deps/gpui_wgpu)
            └── gpui_windows v0.1.0 (native/vendor/zed-deps/gpui_windows)
            [build-dependencies]
            └── gpui_macos v0.1.0 (native/vendor/zed-deps/gpui_macos) (*)
            [dev-dependencies]
            ├── gpui-component v0.5.2 (native/vendor/gpui-component) (*)
            ├── gpui_macos v0.1.0 (native/vendor/zed-deps/gpui_macos) (*)
            └── gpui_macros v0.1.0 (proc-macro) (native/vendor/zed-deps/gpui_macros)
                ├── gpui v0.2.2 (native/vendor/gpui) (*)
                └── gpui-component v0.5.2 (native/vendor/gpui-component) (*)
[dev-dependencies]
└── sum_tree v0.1.0 (native/vendor/zed-deps/sum_tree) (*)

$ cargo tree -i ztracing
ztracing v0.1.0 (native/vendor/zed-deps/ztracing)
└── sum_tree v0.1.0 (native/vendor/zed-deps/sum_tree)
    └── gpui v0.2.2 (native/vendor/gpui)
        ├── gpui-component v0.5.2 (native/vendor/gpui-component)
        │   └── gpui-vendor-probe v0.0.0 (native/vendor/probe)
        ├── gpui-component-assets v0.5.1 (native/vendor/gpui-component-assets)
        │   └── gpui-component v0.5.2 (native/vendor/gpui-component) (*)
        ├── gpui-vendor-probe v0.0.0 (native/vendor/probe)
        ├── gpui_macos v0.1.0 (native/vendor/zed-deps/gpui_macos)
        │   └── gpui_platform v0.1.0 (native/vendor/zed-deps/gpui_platform)
        │       └── gpui-vendor-probe v0.0.0 (native/vendor/probe)
        │       [dev-dependencies]
        │       └── gpui v0.2.2 (native/vendor/gpui) (*)
        ├── gpui_platform v0.1.0 (native/vendor/zed-deps/gpui_platform) (*)
        ├── gpui_wgpu v0.1.0 (native/vendor/zed-deps/gpui_wgpu)
        └── gpui_windows v0.1.0 (native/vendor/zed-deps/gpui_windows)
        [build-dependencies]
        └── gpui_macos v0.1.0 (native/vendor/zed-deps/gpui_macos) (*)
        [dev-dependencies]
        ├── gpui-component v0.5.2 (native/vendor/gpui-component) (*)
        ├── gpui_macos v0.1.0 (native/vendor/zed-deps/gpui_macos) (*)
        └── gpui_macros v0.1.0 (proc-macro) (native/vendor/zed-deps/gpui_macros)
            ├── gpui v0.2.2 (native/vendor/gpui) (*)
            └── gpui-component v0.5.2 (native/vendor/gpui-component) (*)
```

*(Absolute paths shortened to `native/vendor/…`; nothing else altered.)*

The alleged chain therefore reads, end to end:
`gpui-component → gpui → sum_tree → ztracing → zlog`.

---

## `gpui`'s feature list

Verbatim from `native/vendor/gpui/Cargo.toml`:

```toml
[features]
default = ["font-kit", "wayland", "x11", "windows-manifest"]
test-support = [
    "leak-detection",
    "collections/test-support",
    "http_client/test-support",
    "wayland",
    "x11",
    "proptest",
]
bench = ["test-support", "dep:criterion", "dep:hdrhistogram"]
inspector = ["gpui_macros/inspector"]
leak-detection = ["backtrace"]
wayland = []
x11 = [
    "scap?/x11",
]
screen-capture = [
    "scap",
]
windows-manifest = ["dep:embed-resource"]
input-latency-histogram = ["dep:hdrhistogram"]
profiler = []
```

Answering the specific questions from the item:

* **`test-support` exists** ✅ and **its first entry is `leak-detection`** ✅ — so
  spec §12's "leak detection on in every test" is satisfied by
  `gpui = { path = "…", features = ["test-support"] }` in `[dev-dependencies]`;
  no second flag needed. `leak-detection` itself is `["backtrace"]`, i.e. it
  turns on the `backtrace` crate so leaked entities report an allocation site.

  Not taken from the manifest alone — the probe carries an off-by-default
  `gpui-test-support` feature so the chain can be resolved and checked:

  ```
  $ cargo tree --features gpui-test-support -e features -i backtrace | grep -n 'leak-detection\|test-support'
  9:│   │   │       │       └── gpui-vendor-probe feature "gpui-test-support" (command-line)
  35:│   │   │   └── gpui feature "leak-detection"
  36:│   │   │       └── gpui feature "test-support"
  ```

  i.e. `test-support` → `leak-detection` → `backtrace`, confirmed by the
  resolver.
* **`inspector`** ✅ present (forwards to `gpui_macros/inspector`).
  `gpui-component` has a matching `inspector` feature that turns on both.
* **`screen-capture`** ✅ present (pulls the `zed-scap` git fork, rev-pinned).
* **`input-latency-histogram`** ✅ present (`dep:hdrhistogram`).
* **`profiler`** ✅ present (empty feature — a pure cfg gate).

`test-support` pulls `proptest` from a rev-pinned git fork
(`proptest-rs/proptest` @ `3dca198a…`), so the first test build needs network
access to that repo.

One caveat when enabling `test-support`: it also forces `wayland` and `x11` on,
which are no-ops on macOS but do widen the resolved graph.

---

## What the root `native/Cargo.toml` must add

Paste-ready. Nothing else is required: **there is no `[patch]` section and no
`.cargo/config.toml`**, because every vendored crate is a plain `path`
dependency with a fully de-inherited manifest (see
[Deviations](#deviations-from-upstream)).

```toml
[workspace]
members = ["crates/*"]
exclude = ["vendor"]
resolver = "2"
```

```toml
[workspace.dependencies]
# Vendored + SHA-pinned; see native/vendor/PINNED.md.
gpui = { path = "vendor/gpui" }
gpui_macros = { path = "vendor/zed-deps/gpui_macros" }
gpui_platform = { path = "vendor/zed-deps/gpui_platform", features = ["font-kit", "runtime_shaders"] }
gpui-component = { path = "vendor/gpui-component" }
gpui-component-assets = { path = "vendor/gpui-component-assets" }
gpui-component-macros = { path = "vendor/gpui-component-macros" }
```

And in a member crate:

```toml
[dependencies]
gpui.workspace = true
gpui_platform.workspace = true
gpui-component.workspace = true

[dev-dependencies]
gpui = { workspace = true, features = ["test-support"] }
```

**These stanzas were tested, not just written.** A `native/`-shaped scratch root
(the exact `[workspace]` + `[workspace.dependencies]` above, `crates/app` as the
only member, `vendor` symlinked at this tree) resolved and then `cargo check`ed a
member crate whose public function takes `&mut gpui::App` and returns
`gpui_component::button::Button`, with the `test-support` dev-dependency line
present:

```
$ cargo metadata --format-version 1        # RESOLVE OK
$ cargo check -p crowbar-ui-probe
    Checking crowbar-ui-probe v0.0.0 (…/crates/app)
    Finished `dev` profile [unoptimized + debuginfo] target(s) in 5m 35s
```

The scratch root was used because `native/Cargo.toml` belongs to another work
item; nothing outside `native/vendor/` was created or modified.

Notes on those choices:

* `exclude = ["vendor"]` — `native/vendor/Cargo.toml` is a second workspace root
  (it exists so the probe can build standalone). Cargo tolerates the nesting
  either way, but `exclude` is the form that was actually tested.
* `gpui_platform` is the crate that owns `application()`; `gpui` alone gives you
  the framework but no window system. Enable `runtime_shaders` unless you are
  prepared to require a full Xcode install on every build machine — see
  [System build dependencies](#system-build-dependencies).
* `gpui-component`'s syntax highlighting is behind features. For spec §10.1's
  tree-sitter highlighter, add the languages you want:
  `gpui-component = { workspace = true, features = ["tree-sitter-rust", "tree-sitter-typescript", …] }`,
  or `features = ["tree-sitter-languages"]` for all 35 grammars (a large build).
  `decimal` and `inspector` are the other notable features.
* Do **not** add `gpui = { features = ["default"] }` thinking you need it —
  `default` is already on (`font-kit`, `wayland`, `x11`, `windows-manifest`);
  the last three are inert on macOS.

---

## System build dependencies

**macOS** (what this was built on):

* Xcode Command Line Tools — `cbindgen` in `gpui_macos/build.rs` generates
  `scene.h` from gpui's Rust types, and the AppKit/Metal crates need the SDK.
  The graph also compiles `bindgen`/`clang-sys`, so **libclang must be present**;
  the CLT ships it, but a container image that installs only `rustup` will not.
* **Full Xcode is required *only* if you turn `runtime_shaders` off.** Without
  that feature `gpui_macos/build.rs` shells out to
  `xcrun -sdk macosx metal` / `metallib` to precompile `shaders.metal`, and the
  Metal compiler does not ship with the Command Line Tools alone. With
  `runtime_shaders` on (the setting used here, and the one `gpui-component`'s
  own workspace uses) the shader source is stitched into the binary and
  compiled by the Metal runtime at startup. Keep it on.
  (On this machine the Metal toolchain *is* installed — `xcrun -f metal`
  resolves — so a `cannot execute tool 'metal'` failure here would not be a
  missing 688 MB component and you should look elsewhere. `runtime_shaders`
  makes the question moot for CI, which is the point of keeping it on.)
* No Homebrew packages needed.

**Linux** (not built here — this is the list the manifests imply, unverified):

* `libxkbcommon-dev`, `libx11-dev` + the `x11rb`/`xim` stack, `libwayland-dev`
  and `wayland-protocols` — `gpui_linux` compiles both backends, and both
  `x11` and `wayland` are default features of `gpui`.
* `libasound2-dev` / ALSA is *not* needed (that lives in Zed crates we did not
  vendor).
* `vulkan` loader + `libgbm`/`libdrm` for `gpui_wgpu`'s blade/wgpu backend.
* `pkg-config`, `cmake`, and a C toolchain for the `resvg`/`fontconfig`/
  `freetype` chain.
* `libssl-dev` only if `reqwest_client` is actually linked (it is a dev-dep of
  `gpui`, so normally not).

**Windows**: `gpui_windows` is vendored and resolves, but has never been built
here.

---

## Deviations from upstream

Every vendored `Cargo.toml` has been **de-inherited**: each
`something.workspace = true` was replaced with the concrete value from the
upstream workspace root it was inheriting from. Nothing else in any manifest
changed, and no `.rs` file was touched.

Why: a crate that says `edition.workspace = true` can only be built inside a
workspace that defines it. If the vendored crates kept their inheritance, the
root `native/Cargo.toml` would have to carry ~80 of Zed's
`[workspace.dependencies]` entries verbatim — and Cargo flatly refuses a `path`
dependency that points into a *nested* workspace (`package … is a member of the
wrong workspace`). De-inheriting makes each crate self-contained, so it is a
plain `path` dep from anywhere and the root workspace needs the six lines above
and nothing more.

Concretely, the rewrite did four things:

1. `edition.workspace = true` → `edition = "2024"`; `publish.workspace = true`
   → `publish = false`.
2. `foo.workspace = true` / `foo = { workspace = true, … }` → the merged
   concrete spec (workspace base + local `features` union + local
   `optional` / `default-features` overrides).
3. In-repo `path` values rewritten for the `gpui/` + `zed-deps/` layout.
4. `[lints] workspace = true` → the resolved `[lints.rust]` / `[lints.clippy]`
   tables written out inline.

Three further edits, each load-bearing:

* **`gpui-component`'s own workspace pinned `gpui` to a floating ref** —
  literally `gpui = { git = "https://github.com/zed-industries/zed" }`, no
  `rev`, no `branch`. Every such entry was re-pointed at the vendored path. This
  is the single most important edit in the whole job: left alone it would have
  meant a dependency that silently re-resolves to Zed's `main` on any
  `cargo update`.
* `gpui-component`'s `readme = "../../README.md"` pointed outside the crate;
  the file was copied in and the key re-pointed. `LICENSE-APACHE` was copied
  into `gpui-component-assets/` and `gpui-component-macros/`, which carry none
  of their own (the repo-root Apache-2.0 text covers them).
* `zed-deps/gpui_web/examples/` was **deleted**. It contained a nested
  `hello_web` package with its own empty `[workspace]` table — a second
  workspace root inside `native/vendor/`, which Cargo rejects outright
  (`multiple workspace roots found in the same workspace`) — and its relative
  paths no longer resolved in the flattened layout. It is a wasm demo; nothing
  depends on it.

### How Zed licences its source — and what that forced here

Zed carries **no per-file licence headers**. Licensing is per-crate: a
`license = "…"` key in each `Cargo.toml`, plus a `LICENSE-APACHE` and/or
`LICENSE-GPL` in the crate directory that is a **symlink to the repo root**.

A plain `cp -R` therefore leaves a dangling link and no licence text at all. The
copy here dereferences (`shutil.copytree(..., symlinks=False)`, i.e. `cp -RL`),
so:

```
$ find native/vendor -type l | wc -l          # 0 symlinks
$ find native/vendor -name 'LICENSE*' | wc -l # 31 real files
$ head -2 native/vendor/zed-deps/ztracing/LICENSE-GPL
GNU GENERAL PUBLIC LICENSE
Version 3, 29 June 2007
```

Because the `license` key is the only per-crate signal, it is also the *only*
thing to read for provenance — and reading it is what turned up the GPL crates
above. In particular `path` is **GPL-3.0-or-later**, not Apache-2.0; so are
`zlog`, `ztracing` and `ztracing_macro`. `gpui`, `collections`, `refineable`
and `util` are Apache-2.0. (Note also that crates.io's `gpui` 0.2.2 is dual
`Apache-2.0 OR MIT`, while the git tree at `1a246efd…` declares plain
`Apache-2.0` — another small divergence between the two sources.)

### After every re-vendor, this must hold

```
grep -rn 'workspace = true' native/vendor --include=Cargo.toml   # → nothing
grep -rn 'git = ' native/vendor --include=Cargo.toml             # → every hit is rev-pinned
```

The second is the no-floating-refs invariant. Read it by eye rather than
piping through `grep -v 'rev = '`: `gpui_windows` declares `scap` as a
`[target.'…'.dependencies.scap]` **sub-table**, so its `git` and `rev` land on
separate lines and a naive filter reports a false positive. The rev-pinned git deps that
legitimately remain (all inherited from Zed's own workspace, all either optional
or dev-only on macOS) are:

| dep | repo | rev |
|---|---|---|
| `zed-font-kit` | `zed-industries/font-kit` | `94b0f28166665e8fd2f53ff6d268a14955c82269` |
| `zed-scap` | `zed-industries/scap` | `4afea48c3b002197176fb19cd0f9b180dd36eaac` |
| `zed-reqwest` | `zed-industries/reqwest.git` | `c15662463bda39148ba154100dd44d3fba5873a4` |
| `proptest` | `proptest-rs/proptest` | `3dca198a8fef1b32e3a66f1e1897c955b4dc5b5b` |
| `xim` | `zed-industries/xim-rs.git` | `16f35a2c881b815a2b6cdfd6687988e84f8447d8` |

### Zed's `[patch.crates-io]` costs us nothing — verified

Zed's root manifest carries a `[patch.crates-io]` block with ten forks:
`async-process`, `async-task`, `windows-capture`, `calloop`, `livekit`,
`libwebrtc`, `webrtc-sys`, `notify`, `notify-types`, and (via
`[workspace.dependencies]`) the `font-kit` / `scap` pair.

There is a live worry that pinning `gpui` from Zed drags that whole table into
the consuming workspace, so that a build touching only the UI ends up fetching
livekit, libyuv and the livekit protocol submodule. **That is not what happens
to a consumer.** Cargo honours `[patch]` only from the *top-level workspace
manifest*; a dependency's own `[patch]` table is ignored. The observation that
livekit gets fetched is what you see when you build *inside* Zed's checkout,
where Zed's manifest **is** the workspace root.

Audited against our committed `Cargo.lock`:

| crate Zed patches | where ours resolves from |
|---|---|
| `async-process` | crates.io (upstream) |
| `async-task` | crates.io (upstream) |
| `windows-capture` | crates.io (upstream) |
| `calloop` | crates.io (upstream) |
| `notify` | crates.io (upstream) |
| `notify-types` | crates.io (upstream) |
| `livekit` | **absent from the lock** |
| `libwebrtc` | **absent from the lock** |
| `webrtc-sys` | **absent from the lock** |
| `libyuv` | **absent from the lock** |

Zero patch entries adopted, zero webrtc, and the six that do appear come from
upstream crates.io rather than Zed's forks. The only git sources in the entire
lock are the five listed above, all rev-pinned, all optional / dev / non-macOS.

Do **not** import Zed's `[patch]` table "for parity". Beyond the webrtc weight,
its `calloop` entry has **no rev at all** — copying it would reintroduce exactly
the floating ref this vendoring exists to eliminate.

### The one-source rule

If a workspace vendors copies of Zed's shared support crates *and also* sources
`gpui` from a Zed checkout, Cargo refuses outright:

```
error: package collision in the lockfile: packages collections v0.1.0 (…/vendor/collections)
and collections v0.1.0 (…/zed/crates/collections) are different, but only one can be
written to lockfile unambiguously
```

Whatever supplies `gpui` must also supply `collections`, `gpui_util`,
`refineable`, `gpui_macros` and the rest. This tree does: every one of the 29
comes from `native/vendor/`, nothing points at a Zed checkout, which is why the
`--locked` build below is clean. Keep it that way — do not add a second source
for any of these names.

---

## Proof: the pair builds together

The probe (`native/vendor/probe/`) is one throwaway binary that names a `gpui`
type and a `gpui-component` type in both directions across two function
signatures, and runs a real `gpui_platform::application()` — so the two crates
are genuinely *linked*, not merely co-resolved.

```
$ cd native/vendor
$ cargo build --release -p gpui-vendor-probe --locked
    Finished `release` profile [optimized] target(s) in 3.34s
warning: the following packages contain code that will be rejected by a future version of Rust: block v0.1.6
note: to see what the problems were, use the option `--future-incompat-report`, or run `cargo report future-incompatibilities --id 1`

$ cargo tree --depth 1 -p gpui-vendor-probe
gpui-vendor-probe v0.0.0 (native/vendor/probe)
├── gpui v0.2.2 (native/vendor/gpui)
│   [build-dependencies]
├── gpui-component v0.5.2 (native/vendor/gpui-component)
└── gpui_platform v0.1.0 (native/vendor/zed-deps/gpui_platform)

$ file target/release/gpui-vendor-probe
target/release/gpui-vendor-probe: Mach-O 64-bit executable arm64
```

Every entry in that tree is a local `path`, so `gpui` and `gpui-component` both
come from the vendored sources at the recorded SHAs. `--locked` passing means
the committed `Cargo.lock` *is* the resolution — nothing re-resolved.

Cold-build facts, for whoever budgets CI: 455 crates compiled from scratch,
988 packages in the lock (that count spans Linux/Windows/wasm targets too,
because Cargo resolves every target even when it only builds one), ~1.9 GB of
`target/`, roughly 35 minutes wall on an M-series laptop that was also running
other builds. `target/` is gitignored via `native/vendor/.gitignore`.

`Cargo.lock` **is** committed. It is the second half of the pin: the SHAs fix
the two vendored trees, and the lock fixes the ~960 crates.io packages beneath
them.

The one warning is upstream and pre-existing: `block v0.1.6` (an Objective-C
shim reached through `cocoa`) uses a pattern a future rustc will reject. It is
not ours to fix and does not affect 1.96.0.

The `--locked` output above is a warm rebuild. The honest first-run caveat: on
the very first cold build the probe failed to compile — `Button::primary()`
needs `gpui_component::button::ButtonVariants` in scope. That was a bug in the
probe's own source (one missing `use`), not in the pin: every one of the 29
vendored crates compiled cleanly before the probe was reached. Fixed and
rebuilt; the output above is the result.

---

## Upstream `skills/` (not installed here)

`gpui-component` ships two Claude skills at the pinned SHA. **They were
deliberately not installed** — a different item owns that:

* `skills/gpui/SKILL.md` + `skills/gpui/references/` (22 reference files:
  `action`, `async`, `context`, `element*`, `entity*`, `event`, `focus-handle`,
  `global`, `layout-style`, `test*`)
* `skills/gpui-component/SKILL.md` + `skills/gpui-component/references/`
  (`usage.md`, `style-guide.md`)

Fetch them from
`longbridge/gpui-component@88f102d13654fe25aa2fede076274b6b751a3704`, i.e. the
same SHA pinned above — which is also the SHA queue item 0.3 already used, so
the installed skills and this vendored source are describing the same code.

---

## How to re-pin

1. Pick the new `gpui-component` commit **first** — it is the constraint. Then
   read which Zed revision *it* resolves to; never choose a `gpui` SHA
   independently:

   ```
   git ls-remote https://github.com/longbridge/gpui-component.git refs/heads/main
   curl -sSL https://raw.githubusercontent.com/longbridge/gpui-component/<NEW_GC_SHA>/Cargo.lock \
     | grep -m1 'git+https://github.com/zed-industries/zed#'
   ```

   The fragment after `#` is the new Zed SHA.

2. Fetch both trees at those exact SHAs (tarballs are fine and avoid a 1 GB
   clone):

   ```
   curl -sSL -o zed.tar.gz https://codeload.github.com/zed-industries/zed/tar.gz/<NEW_ZED_SHA>
   curl -sSL -o gc.tar.gz  https://codeload.github.com/longbridge/gpui-component/tar.gz/<NEW_GC_SHA>
   ```

3. Re-measure the closure before copying anything — Zed splits and merges these
   crates often (`gpui_platform`, `gpui_web` and `gpui_wgpu` did not exist a few
   months ago). Walk in-repo `path` deps transitively from `crates/gpui`,
   `crates/gpui_platform` and `crates/gpui_macros`, including
   `[build-dependencies]` and every `[target.'cfg(…)'.dependencies]` table.
   Diff the result against the crate table above.

4. Copy each closure crate to `native/vendor/gpui/` (for `gpui` itself) or
   `native/vendor/zed-deps/<package-name>/`, and `crates/{ui,macros,assets}`
   from gpui-component to `native/vendor/gpui-component{,-macros,-assets}/`.
   Resolve the `LICENSE-*` symlinks into real files (`cp -L`).

5. Re-apply the four de-inheritance rewrites and the three special edits from
   [Deviations](#deviations-from-upstream). The script that did it the first
   time is not checked in — it is ~250 lines of Python driving `tomllib` plus a
   line-oriented rewriter, and it is quicker to re-derive than to maintain
   against Zed's manifest churn. The two `grep` invariants above are the
   acceptance test.

6. Update this file: SHAs, dates, `rust-toolchain.toml`, the crate table, the
   licence-provenance list, `gpui`'s `[features]`, and the git-rev table.

7. `cargo build --release -p gpui-vendor-probe` from `native/vendor/`, then
   build the real app. If the probe compiles but the app does not, the break is
   in our code, not the pin.

8. **Re-test the crates.io route while you are here.** Point a scratch copy of
   `gpui-component` at `gpui = "<latest published>"` +
   `gpui_macros = { version = "<same>", package = "gpui-macros" }` and
   `cargo check`. Today that is 338 errors (see
   [Why not crates.io](#why-not-cratesio--measured-not-assumed)); the day it is
   zero, delete this whole directory and take the published crates instead. Do
   not skip this step on the assumption that the answer is still no.
