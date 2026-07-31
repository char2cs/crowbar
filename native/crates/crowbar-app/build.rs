#![forbid(unsafe_code)]

//! Discovers the measurable surfaces — and their layout tests — so that adding
//! one is **adding a file**.
//!
//! # Why this exists
//!
//! Phase 2 puts four components in four parallel worktrees, and before this the
//! binary dispatched `--surface` from one `match` — five of them, actually: the
//! name, the root anchor, the unmodelled flags, the element tree and the
//! caption's per-surface extras. Four workers each adding an arm to all five is
//! four conflicts in the same five hunks, in files none of them own.
//!
//! Rust has no way to find a module nobody declared: `mod x;` is a line in a
//! shared file, and `inventory`/`linkme`-style distributed slices still need
//! one. A build script does not. It reads `src/surfaces/`, writes a `#[path]`
//! module declaration per file and a registry slice over them, and
//! `src/surfaces/mod.rs` `include!`s the result — so a new surface touches
//! **its own new file and nothing else**, and two surfaces landing at once
//! cannot conflict because neither wrote the list.
//!
//! # What it costs, stated rather than discovered later
//!
//! * The module list is generated, so `git grep 'mod dropdown_menu'` finds
//!   nothing. `src/surfaces/mod.rs` says so in its own docs, which is the first
//!   place anyone looks.
//! * The order is **sorted by file name**, not by importance. `--help` lists the
//!   surfaces in that order, and the default surface is named by path in
//!   `row_surface.rs` rather than by being first.
//! * `rerun-if-changed` is the directory, so cargo re-runs this when a file is
//!   added or removed. Edits inside a file are tracked by rustc's own dep-info,
//!   because the modules are compiled normally once declared.
//! * A file that is not a surface — a shared helper under `src/surfaces/` —
//!   would be declared as a module and then fail to compile for want of a
//!   `SURFACE`. That is loud, and it is the reason `src/surfaces/` holds
//!   surfaces and nothing else.
//!
//! # The second directory, and why it is the same problem
//!
//! `src/row_layout/` gets the identical treatment, added in P3.5 after the same
//! failure mode played out a second time. `row_layout.rs` had grown to 4083
//! lines holding eight per-surface `mod` blocks of `#[gpui::test]`s, and every
//! Tier B worker appended a ninth **at the end of it** — one place, from
//! several worktrees, which conflicted on four consecutive merges. Each
//! resolution was mechanical and each one cost a round trip.
//!
//! So the same mechanism, minus the registry: this writes module declarations
//! only, `src/row_layout.rs` `include!`s them, and it keeps the shared harness
//! the modules reach through `super`. A surface's layout tests are its own file
//! and nothing else, and two of them landing at once cannot conflict.
//!
//! The differences from `src/surfaces/`, both deliberate:
//!
//! * The modules are **private and not `pub`**: they are `#[cfg(test)]` — the
//!   attribute is on `row_layout.rs` and covers everything the `include!`
//!   brings in — and nothing outside them uses their contents.
//! * There is **no registry**, because there is nothing to register. A file
//!   here declares nothing; it just has to compile and its tests have to pass.
//!   The corresponding "did a file appear or vanish by accident" check is
//!   `surface.rs`'s hand-written name list, which already covers the surfaces
//!   these measure.

use std::error::Error;
use std::fmt::Write as _;
use std::fs;
use std::path::{Path, PathBuf};

/// Where the surfaces live. Every `.rs` file here except `mod.rs` is one.
const SURFACE_DIR: &str = "src/surfaces";

/// The generated file `src/surfaces/mod.rs` includes.
const GENERATED: &str = "surfaces.rs";

/// Where the per-surface layout tests live. Every `.rs` file here is one
/// surface's worth, and none of them is `mod.rs` — `src/row_layout.rs` is the
/// module file.
const LAYOUT_DIR: &str = "src/row_layout";

/// The generated file `src/row_layout.rs` includes.
const LAYOUT_GENERATED: &str = "row_layout_mods.rs";

fn main() -> Result<(), Box<dyn Error>> {
    let manifest = PathBuf::from(std::env::var("CARGO_MANIFEST_DIR")?);
    let out = PathBuf::from(std::env::var("OUT_DIR")?);
    let dir = manifest.join(SURFACE_DIR);

    println!("cargo::rerun-if-changed={}", dir.display());

    let modules = discover(&dir)?;
    if modules.is_empty() {
        // A registry with nothing in it would make `--surface` reject every
        // word and `--help` list nothing, which reads like a parser bug three
        // steps away from the cause. Fail here instead, where the cause is.
        return Err(format!("{} declares no surfaces", dir.display()).into());
    }

    fs::write(out.join(GENERATED), render(&dir, &modules))?;

    let layout = manifest.join(LAYOUT_DIR);
    println!("cargo::rerun-if-changed={}", layout.display());

    let layout_modules = discover(&layout)?;
    if layout_modules.is_empty() {
        // A test file that compiles no tests passes, silently and for ever.
        // The one way this directory empties itself is a mistake, so say so
        // here rather than let `cargo test` report a smaller number nobody
        // reads.
        return Err(format!("{} declares no layout tests", layout.display()).into());
    }

    fs::write(
        out.join(LAYOUT_GENERATED),
        declare(&layout, &layout_modules, ""),
    )?;
    Ok(())
}

/// Every surface module name in `dir`, sorted, so the generated file is stable
/// across machines and filesystems.
fn discover(dir: &Path) -> Result<Vec<String>, Box<dyn Error>> {
    let mut modules = Vec::new();
    for entry in fs::read_dir(dir)? {
        let path = entry?.path();
        if path.extension().is_none_or(|ext| ext != "rs") {
            continue;
        }
        let Some(stem) = path.file_stem().and_then(|stem| stem.to_str()) else {
            continue;
        };
        if stem == "mod" {
            continue;
        }
        modules.push(stem.to_owned());
    }
    modules.sort();
    Ok(modules)
}

/// A `#[path]` module declaration per discovered file, at the given visibility
/// (`""` for private, `"pub "` for public).
///
/// The `#[path]` is absolute because the declarations land inside an `include!`,
/// and a relative module path there resolves against a directory that is not
/// obviously either file's.
fn declare(dir: &Path, modules: &[String], vis: &str) -> String {
    let mut out = format!(
        "// @generated by build.rs from {}/. Do not edit; add a file instead.\n",
        dir.file_name().unwrap_or_default().to_string_lossy(),
    );
    for module in modules {
        let _ = writeln!(
            out,
            "#[path = {:?}]\n{vis}mod {module};",
            dir.join(format!("{module}.rs")).display().to_string(),
        );
    }
    out
}

/// The generated module declarations and the registry over them.
fn render(dir: &Path, modules: &[String]) -> String {
    let mut out = declare(dir, modules, "pub ");
    out.push_str(
        "\n/// Every surface the binary can draw, sorted by module name.\n\
         ///\n\
         /// Generated, so a surface reaches this list by existing.\n\
         pub static ALL: &[&crate::surface::Surface] = &[\n",
    );
    for module in modules {
        let _ = writeln!(out, "    &{module}::SURFACE,");
    }
    out.push_str("];\n");
    out
}
