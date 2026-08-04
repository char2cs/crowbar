//! Which Crowbar home this process should use, and whether it is an override
//! of the production default.
//!
//! Mirrors the Go daemon's own resolution order, with one addition Go does not
//! have: a dev-build fallback so a plain `cargo run` never collides with the
//! production `~/.crowbar`. Ported from `desktop/src-tauri/src/sidecar/mod.rs`'s
//! `crowbar_home()` (S0.3); the byte-identical *socket path* derivation itself
//! now lives in `crowbar_client::socket`, which did not exist when that
//! function was first written — see [`crate::socket`].
//!
//! 1. `CROWBAR_HOME` env override (must be non-empty) — override.
//! 2. Debug builds only: `<repo root>/.crowbar`, where the repo root is derived
//!    from *this crate's own* `CARGO_MANIFEST_DIR` at compile time
//!    (`native/crates/crowbar-sidecar` sits three directories under the repo
//!    root) — override. This keeps a dev instance of either binary off the
//!    production home regardless of which one links this crate.
//! 3. `$HOME` (`%USERPROFILE%` on Windows) joined with `.crowbar` — production.
//!
//! The decision itself ([`resolve_from`]) is pure, in the same shape
//! `crowbar_client::socket` uses: no test needs to mutate the process
//! environment (this crate `#![forbid(unsafe_code)]`, and edition 2024 makes
//! `std::env::set_var` an `unsafe fn`, so no test *could* even if it wanted
//! to). [`resolve`] is the one impure edge that reads the three inputs and
//! hands them to it.

use std::path::{Path, PathBuf};

/// The directory a non-overridden home lives in, under the user's home.
/// Duplicated from `crowbar_client::socket`'s private constant of the same
/// name (which cannot export it: only the *derivation function* is public
/// API) — this is a stable, one-word literal, not the hash contract that
/// module's own doc comment warns against re-deriving.
const HOME_DIR_NAME: &str = ".crowbar";

/// The resolved Crowbar home, and whether it is an override of the production
/// default.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct CrowbarHome {
    /// The home directory itself, e.g. `~/.crowbar` or `$CROWBAR_HOME`.
    pub path: PathBuf,
    /// Whether `path` came from an override (`CROWBAR_HOME` or the dev-build
    /// fallback) rather than the production default. Spawning passes
    /// `CROWBAR_HOME` to the child only when this is `true` — production keeps
    /// the env unset so the daemon resolves its own default, matching what it
    /// would do standalone.
    pub overridden: bool,
}

/// The pure inputs [`resolve_from`] decides between. Borrowed, like
/// `crowbar_client::socket::Location`, so a test can exercise any combination
/// without touching the process environment.
#[derive(Debug, Clone, Copy)]
pub struct Env<'a> {
    /// `$CROWBAR_HOME`, already filtered to `None` for an unset *or empty*
    /// variable — Go tests `override != ""`, and this crate's contract with it
    /// depends on agreeing.
    pub crowbar_home: Option<&'a Path>,
    /// The dev-build repo-root fallback, precomputed from this crate's own
    /// `CARGO_MANIFEST_DIR` — `None` outside `debug_assertions`.
    pub dev_repo_root: Option<&'a Path>,
    /// `$HOME` (`%USERPROFILE%` on Windows).
    pub user_home: Option<&'a Path>,
}

impl Env<'_> {
    /// Applies the three-step order this module documents.
    ///
    /// The empty-string check on `crowbar_home` lives *here*, not only in
    /// [`resolve`]'s impure edge: a test that builds an `Env` directly (as
    /// every test in this module's own suite does, to avoid mutating the
    /// process environment — this crate `#![forbid(unsafe_code)]`, and
    /// edition 2024 makes `std::env::set_var` an `unsafe fn`) must see the
    /// same rule a real caller does, or the pure half would be testing a
    /// different policy than the one that actually ships.
    #[must_use]
    pub fn resolve(&self) -> Option<CrowbarHome> {
        if let Some(home) = self
            .crowbar_home
            .filter(|home| !home.as_os_str().is_empty())
        {
            return Some(CrowbarHome {
                path: home.to_path_buf(),
                overridden: true,
            });
        }
        if let Some(root) = self.dev_repo_root {
            return Some(CrowbarHome {
                path: root.join(HOME_DIR_NAME),
                overridden: true,
            });
        }
        self.user_home.map(|home| CrowbarHome {
            path: home.join(HOME_DIR_NAME),
            overridden: false,
        })
    }
}

/// Resolves the home this process should use, following the three-step order
/// this module documents.
///
/// Returns `None` only when neither `CROWBAR_HOME` nor the platform's own home
/// variable (`HOME`/`USERPROFILE`) is set — a machine with neither is not one
/// this crate can place a home on, and the daemon-log path falls back to the
/// system temp directory in that case (see [`daemon_log_path`]).
#[must_use]
pub fn resolve() -> Option<CrowbarHome> {
    // Emptiness is filtered inside `Env::resolve`, not here — see that
    // method's own doc comment for why the check has to live in the pure
    // half.
    let crowbar_home = std::env::var_os("CROWBAR_HOME");
    let crowbar_home = crowbar_home.as_deref().map(Path::new);
    let user_home = user_home_var();
    let user_home = user_home.as_deref().map(Path::new);

    #[cfg(debug_assertions)]
    let dev_repo_root = repo_root();
    #[cfg(not(debug_assertions))]
    let dev_repo_root = None;

    Env {
        crowbar_home,
        dev_repo_root,
        user_home,
    }
    .resolve()
}

/// `<repo root>/.crowbar`'s root half: three directories above this crate's
/// own manifest (`native/crates/crowbar-sidecar`), which is fixed at compile
/// time regardless of which binary links the crate or where it runs from.
#[cfg(debug_assertions)]
fn repo_root() -> Option<&'static Path> {
    Path::new(env!("CARGO_MANIFEST_DIR"))
        .parent()
        .and_then(Path::parent)
        .and_then(Path::parent)
}

#[cfg(windows)]
fn user_home_var() -> Option<std::ffi::OsString> {
    std::env::var_os("USERPROFILE")
}

#[cfg(not(windows))]
fn user_home_var() -> Option<std::ffi::OsString> {
    std::env::var_os("HOME")
}

/// The daemon's captured stdout/stderr: `<crowbar home>/logs/daemon.log`. This
/// is the only place a Go panic's stack trace survives in a packaged app — the
/// child's pipes are otherwise connected to nothing.
///
/// Falls back to the system temp directory when [`resolve`] finds no home at
/// all, so a log path always exists even on a machine that cannot dial a
/// daemon either (see [`resolve`]'s doc comment).
#[must_use]
pub fn daemon_log_path() -> PathBuf {
    let home = resolve().map_or_else(std::env::temp_dir, |h| h.path);
    home.join("logs").join("daemon.log")
}

#[cfg(test)]
mod tests {
    use std::path::Path;

    use super::{Env, daemon_log_path, resolve};

    fn env<'a>(
        crowbar_home: Option<&'a str>,
        dev_repo_root: Option<&'a str>,
        user_home: Option<&'a str>,
    ) -> Env<'a> {
        Env {
            crowbar_home: crowbar_home.map(Path::new),
            dev_repo_root: dev_repo_root.map(Path::new),
            user_home: user_home.map(Path::new),
        }
    }

    #[test]
    fn an_override_wins_over_the_dev_fallback_and_production() {
        let home = env(Some("/override"), Some("/repo"), Some("/home/someone"))
            .resolve()
            .expect("an override was supplied");
        assert_eq!(home.path, Path::new("/override"));
        assert!(home.overridden);
    }

    #[test]
    fn an_empty_override_falls_through_to_the_dev_repo_root() {
        let home = env(Some(""), Some("/repo"), Some("/home/someone"))
            .resolve()
            .expect("the dev fallback still applies");
        assert_eq!(home.path, Path::new("/repo/.crowbar"));
        assert!(home.overridden, "the dev-repo-root fallback is an override");
    }

    #[test]
    fn without_an_override_the_dev_repo_root_wins_over_production() {
        let home = env(None, Some("/repo"), Some("/home/someone"))
            .resolve()
            .expect("the dev fallback applies");
        assert_eq!(home.path, Path::new("/repo/.crowbar"));
        assert!(home.overridden);
    }

    #[test]
    fn with_neither_override_the_production_home_is_used() {
        let home = env(None, None, Some("/home/someone"))
            .resolve()
            .expect("a user home was supplied");
        assert_eq!(home.path, Path::new("/home/someone/.crowbar"));
        assert!(!home.overridden, "production is never an override");
    }

    #[test]
    fn with_nothing_at_all_there_is_no_home() {
        assert_eq!(env(None, None, None).resolve(), None);
    }

    /// The real edge: `resolve()`'s dev-build branch must land on the actual
    /// repo root — three directories above this crate's own manifest, i.e. two
    /// above `native/` — not merely on *some* absolute `.crowbar` path.
    /// `cfg(test)` implies `debug_assertions`, so that branch is always live
    /// here.
    #[test]
    fn resolve_lands_the_dev_fallback_on_the_real_repo_root() {
        let home = resolve().expect("a dev test binary always resolves a home");
        assert!(home.overridden, "a dev build always overrides");

        let expected = Path::new(env!("CARGO_MANIFEST_DIR"))
            .parent()
            .and_then(Path::parent)
            .and_then(Path::parent)
            .expect("this crate lives three directories under the repo root")
            .join(".crowbar");
        assert_eq!(home.path, expected);
    }

    #[test]
    fn the_daemon_log_path_sits_under_logs_in_the_resolved_home() {
        let path = daemon_log_path();
        assert!(path.ends_with("logs/daemon.log"), "{}", path.display());
    }
}
