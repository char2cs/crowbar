//! Where the daemon binary (`crowbar-api`) itself lives.
//!
//! This did not exist in `desktop/src-tauri/src/sidecar/mod.rs`: that code
//! never had to answer the question, because `tauri_plugin_shell`'s
//! `Shell::sidecar` answered it — it resolves `<dir of the running
//! executable>/crowbar-api`, the bare name `tauri-build`'s `copy_binaries`
//! step produces by stripping the `-<target-triple>` suffix off whichever
//! `desktop/src-tauri/binaries/crowbar-api-<triple>` matches the build.
//!
//! `crowbar-app` has no such build step, and this crate must not depend on
//! `tauri` — a GPUI binary cannot link it, and §4.2 does not grant this crate
//! an edge to it regardless. So this module reproduces the *outcome* of that
//! resolution (a binary named `crowbar-api` beside the running executable)
//! without the dependency.
//!
//! **S0.8 removed a third candidate** that used to fall back to
//! `<repo root>/desktop/src-tauri/binaries/crowbar-api-<target-triple>` —
//! `fetch-sidecar.sh`'s own output — as a migration convenience: reuse
//! whatever a `make dev-desktop` had already built, so a developer would not
//! have to build the daemon a second time for `crowbar-app`. That convenience
//! was never worth what it cost: `desktop/` is deleted the day `native/`
//! replaces it (§4.3 rule 7), at which point the candidate is permanently
//! absent and the advice this module's error used to print — "run
//! `desktop/scripts/fetch-sidecar.sh`" — sends a developer into a directory
//! that no longer exists, in the *one* message a broken checkout is
//! guaranteed to surface. `api/` is the daemon's actual source and survives
//! the deletion, so [`NotFound`]'s message below tells a developer how to
//! build straight from there instead — the same `go build` invocation
//! `fetch-sidecar.sh` already runs, minus the `desktop/`-owned destination.
//!
//! Search order:
//! 1. `CROWBAR_SIDECAR_BIN` env override — explicit, for tests and packaging.
//! 2. `<dir of the running executable>/crowbar-api[.exe]` — the packaged/dev
//!    convention `tauri-build` already produces for `desktop/`, reproduced
//!    here without linking `tauri`.
//!
//! Each candidate is checked with a real filesystem probe rather than assumed
//! present, so a stale or half-built checkout gets a specific "none of these
//! exist" error rather than a spawn failure with no context.

use std::path::{Path, PathBuf};

/// The bare executable name, platform-adjusted.
#[cfg(windows)]
const BINARY_NAME: &str = "crowbar-api.exe";
#[cfg(not(windows))]
const BINARY_NAME: &str = "crowbar-api";

/// No candidate location held an existing file.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct NotFound {
    /// Every path this search checked, in order — the first thing worth
    /// printing when a launch fails here.
    pub checked: Vec<PathBuf>,
}

impl std::fmt::Display for NotFound {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(
            f,
            "no crowbar-api binary found. Checked: {}. Build one from the \
             daemon's own source (`cd api && go build -tags noEmbed -o <path> \
             ./cmd/crowbar`) and set CROWBAR_SIDECAR_BIN to it, or place a \
             `crowbar-api` beside this executable.",
            self.checked
                .iter()
                .map(|p| p.display().to_string())
                .collect::<Vec<_>>()
                .join(", "),
        )
    }
}

impl std::error::Error for NotFound {}

/// The pure half: given the candidates in priority order, the first one that
/// `exists` reports true for wins. Split out so the priority order itself is
/// unit-testable without touching the real filesystem.
fn first_existing(candidates: &[PathBuf], exists: impl Fn(&Path) -> bool) -> Option<PathBuf> {
    candidates.iter().find(|c| exists(c)).cloned()
}

/// The env-override and packaged-convention candidates, in priority order —
/// the complete set [`locate`] checks (see that function's doc comment for
/// why there are only two, as of S0.8).
fn candidates(env_override: Option<PathBuf>, exe_dir: Option<&Path>) -> Vec<PathBuf> {
    let mut out = Vec::with_capacity(2);
    if let Some(path) = env_override {
        out.push(path);
    }
    if let Some(dir) = exe_dir {
        out.push(dir.join(BINARY_NAME));
    }
    out
}

/// Finds the daemon binary using the search order this module documents.
///
/// # Errors
///
/// [`NotFound`], carrying every path checked, when none of them exists.
pub fn locate() -> Result<PathBuf, NotFound> {
    let env_override = std::env::var_os("CROWBAR_SIDECAR_BIN").map(PathBuf::from);
    let exe_dir = std::env::current_exe()
        .ok()
        .and_then(|exe| exe.parent().map(Path::to_path_buf));

    let checked = candidates(env_override, exe_dir.as_deref());

    match first_existing(&checked, Path::is_file) {
        Some(found) => Ok(found),
        None => Err(NotFound { checked }),
    }
}

#[cfg(test)]
mod tests {
    use std::path::{Path, PathBuf};

    use super::{candidates, first_existing};

    #[test]
    fn the_env_override_is_checked_before_the_exe_dir_candidate() {
        let list = candidates(
            Some(PathBuf::from("/explicit/crowbar-api")),
            Some(Path::new("/app/Contents/MacOS")),
        );
        assert_eq!(
            list,
            vec![
                PathBuf::from("/explicit/crowbar-api"),
                PathBuf::from("/app/Contents/MacOS/crowbar-api"),
            ],
        );
    }

    #[test]
    fn without_an_override_only_the_exe_dir_candidate_is_checked() {
        let list = candidates(None, Some(Path::new("/app/Contents/MacOS")));
        assert_eq!(list, vec![PathBuf::from("/app/Contents/MacOS/crowbar-api")]);
    }

    #[test]
    fn without_a_resolvable_exe_dir_only_the_override_is_checked() {
        let list = candidates(Some(PathBuf::from("/explicit/crowbar-api")), None);
        assert_eq!(list, vec![PathBuf::from("/explicit/crowbar-api")]);
    }

    #[test]
    fn first_existing_picks_the_first_candidate_that_exists() {
        let list = vec![
            PathBuf::from("/does/not/exist/a"),
            PathBuf::from("/does/exist/b"),
            PathBuf::from("/does/exist/c"),
        ];
        let found = first_existing(&list, |p| {
            p == Path::new("/does/exist/b") || p == Path::new("/does/exist/c")
        });
        assert_eq!(found, Some(PathBuf::from("/does/exist/b")));
    }

    #[test]
    fn first_existing_reports_none_when_nothing_exists() {
        let list = vec![PathBuf::from("/a"), PathBuf::from("/b")];
        assert_eq!(first_existing(&list, |_| false), None);
    }

    /// `locate()`'s real edge: in this dev checkout, whether or not a binary
    /// has been built, the *search itself* must not panic and must report a
    /// specific, checkable list on failure — the one thing a developer needs
    /// to fix the problem the error names.
    #[test]
    fn locate_never_panics_and_names_every_checked_path_on_failure() {
        match super::locate() {
            Ok(found) => assert!(found.is_file(), "{}", found.display()),
            Err(not_found) => {
                assert!(!not_found.checked.is_empty());
                assert!(not_found.to_string().contains("CROWBAR_SIDECAR_BIN"));
            }
        }
    }

    /// **S0.8 regression.** The old message's advice — "run
    /// `desktop/scripts/fetch-sidecar.sh`" — became a dead end pointing at a
    /// deleted directory once `desktop/` stopped being a real candidate. This
    /// builds a [`super::NotFound`] directly (rather than depending on
    /// whatever `locate()` happens to find or not find on the machine
    /// actually running the test) so the assertion is about the message
    /// *this module constructs*, unconditionally, not about this checkout's
    /// filesystem state.
    #[test]
    fn the_not_found_message_never_points_at_desktop() {
        let not_found = super::NotFound {
            checked: vec![PathBuf::from("/does/not/exist/crowbar-api")],
        };
        let msg = not_found.to_string();
        assert!(
            !msg.to_ascii_lowercase().contains("desktop"),
            "error message must not send anyone to a deleted directory: {msg}"
        );
        assert!(msg.contains("CROWBAR_SIDECAR_BIN"), "{msg}");
        assert!(msg.contains("api"), "{msg}");
    }
}
