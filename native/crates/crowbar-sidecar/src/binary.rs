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
//! without the dependency, and adds one more candidate: the triple-suffixed
//! file `desktop/scripts/fetch-sidecar.sh` already builds for local
//! development, so a developer who has run it once (directly, or via `make
//! dev-desktop`) does not have to build it again for `crowbar-app`.
//!
//! Search order:
//! 1. `CROWBAR_SIDECAR_BIN` env override — explicit, for tests and packaging.
//! 2. `<dir of the running executable>/crowbar-api[.exe]` — the packaged/dev
//!    convention `tauri-build` already produces for `desktop/`.
//! 3. `<repo root>/desktop/src-tauri/binaries/crowbar-api-<target-triple>` —
//!    `fetch-sidecar.sh`'s own output, for `crowbar-app`'s dev use.
//!
//! Each candidate is checked with a real filesystem probe rather than assumed
//! present, so a stale or half-built checkout gets a specific "none of these
//! exist" error rather than a spawn failure with no context.

use std::path::{Path, PathBuf};

/// This platform's Rust target triple, in the form `fetch-sidecar.sh` names
/// its output with. `None` on a platform that script does not build for
/// (candidate 3 is simply skipped there; candidates 1 and 2 still apply).
#[cfg(all(target_os = "macos", target_arch = "aarch64"))]
const TARGET_TRIPLE: Option<&str> = Some("aarch64-apple-darwin");
#[cfg(all(target_os = "macos", target_arch = "x86_64"))]
const TARGET_TRIPLE: Option<&str> = Some("x86_64-apple-darwin");
#[cfg(all(target_os = "linux", target_arch = "x86_64"))]
const TARGET_TRIPLE: Option<&str> = Some("x86_64-unknown-linux-gnu");
#[cfg(all(target_os = "linux", target_arch = "aarch64"))]
const TARGET_TRIPLE: Option<&str> = Some("aarch64-unknown-linux-gnu");
#[cfg(not(any(
    all(target_os = "macos", target_arch = "aarch64"),
    all(target_os = "macos", target_arch = "x86_64"),
    all(target_os = "linux", target_arch = "x86_64"),
    all(target_os = "linux", target_arch = "aarch64"),
)))]
const TARGET_TRIPLE: Option<&str> = None;

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
            "no crowbar-api binary found. Checked: {}. Set CROWBAR_SIDECAR_BIN, \
             or run desktop/scripts/fetch-sidecar.sh to build one.",
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
/// everything [`locate`] checks except the dev fallback, which needs the repo
/// root threaded in separately (see [`locate`]'s body).
fn candidates(env_override: Option<PathBuf>, exe_dir: Option<&Path>) -> Vec<PathBuf> {
    let mut out = Vec::with_capacity(3);
    if let Some(path) = env_override {
        out.push(path);
    }
    if let Some(dir) = exe_dir {
        out.push(dir.join(BINARY_NAME));
    }
    out
}

/// Candidate 3: `fetch-sidecar.sh`'s own output, given the repo root.
fn dev_fallback_candidate(repo_root: &Path) -> Option<PathBuf> {
    TARGET_TRIPLE.map(|triple| {
        repo_root
            .join("desktop")
            .join("src-tauri")
            .join("binaries")
            .join(format!("crowbar-api-{triple}"))
    })
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

    let mut checked = candidates(env_override, exe_dir.as_deref());
    // `native/crates/crowbar-sidecar` sits three directories under the repo
    // root — the same derivation `crate::home`'s dev fallback uses, kept
    // independent of it deliberately: the daemon *binary* location and the
    // daemon *home* are different questions that happen to share a landmark.
    if let Some(repo_root) = Path::new(env!("CARGO_MANIFEST_DIR"))
        .parent()
        .and_then(Path::parent)
        .and_then(Path::parent)
        && let Some(candidate) = dev_fallback_candidate(repo_root)
    {
        checked.push(candidate);
    }

    match first_existing(&checked, Path::is_file) {
        Some(found) => Ok(found),
        None => Err(NotFound { checked }),
    }
}

#[cfg(test)]
mod tests {
    use std::path::{Path, PathBuf};

    use super::{candidates, dev_fallback_candidate, first_existing};

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

    /// Pinned so a platform this repo does not build a sidecar for (this
    /// function returns `None` there) does not silently claim a path nothing
    /// ever writes to.
    #[test]
    fn the_dev_fallback_candidate_names_fetch_sidecars_own_output_layout() {
        if let Some(candidate) = dev_fallback_candidate(Path::new("/repo")) {
            assert!(candidate.starts_with("/repo/desktop/src-tauri/binaries"));
            assert!(
                candidate
                    .file_name()
                    .unwrap()
                    .to_string_lossy()
                    .starts_with("crowbar-api-")
            );
        }
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
}
