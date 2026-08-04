//! The daemon's unix-socket path, derived from [`crate::home`]'s resolved
//! Crowbar home.
//!
//! The byte-identical arithmetic — the FNV-1a hash, the non-zero-padded `{:x}`
//! format, the 104-byte `sun_path` cap that forces an overridden home onto a
//! short name in the temp directory — lives in `crowbar_client::socket`, which
//! documents itself as the *third* copy of this derivation (the first is the
//! Go daemon's own `overrideSocketPath`; the second, now retired, was
//! `desktop/src-tauri/src/sidecar/mod.rs`'s private `fnv1a64`). This module
//! does not become a fourth: it resolves *which* home applies (that part is
//! this crate's own job — [`crate::home`]) and hands the arithmetic itself to
//! `crowbar_client::socket::Location`.

use std::path::{Path, PathBuf};

use crowbar_client::socket::Location;

use crate::home::{self, CrowbarHome};

/// The name the daemon binds under a non-overridden home. Duplicated from
/// `crowbar_client::socket`'s private constant of the same name for the same
/// reason `crate::home::HOME_DIR_NAME` is: it is a stable literal, not part of
/// the hash contract.
const DEFAULT_SOCKET_NAME: &str = "crowbar.sock";

/// The socket path for a resolved home, given the temp directory an override
/// would be hashed into.
///
/// Pure — no filesystem, no environment — so [`socket_path`]'s decision is
/// unit-testable without a real `CROWBAR_HOME` or `$TMPDIR`.
#[must_use]
pub fn socket_path_for(home: Option<&CrowbarHome>, temp_dir: &Path) -> PathBuf {
    match home {
        Some(CrowbarHome {
            path,
            overridden: true,
        }) => Location {
            crowbar_home: Some(path),
            temp_dir,
            user_home: None,
        }
        .socket_path()
        // Unreachable in practice: `crowbar_home: Some(_)` is the one input
        // `Location::socket_path` cannot fail on. The fallback exists so this
        // stays a total function without `unwrap`/`expect`, which this crate
        // denies outside tests.
        .unwrap_or_else(|_| temp_dir.join(DEFAULT_SOCKET_NAME)),
        Some(CrowbarHome {
            path,
            overridden: false,
        }) => path.join(DEFAULT_SOCKET_NAME),
        None => temp_dir.join(DEFAULT_SOCKET_NAME),
    }
}

/// The daemon's unix-socket path for *this* process's environment: a fixed,
/// well-known location matching the default the daemon itself resolves for
/// `unix://`. A fixed path — rather than a per-process temp path — means every
/// client always knows where to reach the daemon, and the daemon's own
/// stale-socket handling (dial-to-detect + reclaim, unlink on clean shutdown)
/// keeps it healthy across restarts instead of leaving a trail of dead per-PID
/// sockets in the temp dir.
///
/// Production: `~/.crowbar/crowbar.sock`. Overridden homes (`CROWBAR_HOME` env
/// or the dev-build workspace default): a short home-keyed name in the temp
/// dir — the socket cannot live *inside* the override home because macOS caps
/// `sun_path` at 104 bytes and workspace worktree paths routinely exceed it.
#[must_use]
pub fn socket_path() -> PathBuf {
    let temp_dir = std::env::temp_dir();
    socket_path_for(home::resolve().as_ref(), &temp_dir)
}

#[cfg(test)]
mod tests {
    use std::path::{Path, PathBuf};

    use super::{CrowbarHome, socket_path, socket_path_for};

    #[test]
    fn an_overridden_home_lands_in_the_temp_dir_hashed_by_fnv1a64() {
        let home = CrowbarHome {
            path: PathBuf::from("/Users/someone/worktree/.crowbar"),
            overridden: true,
        };
        let temp = Path::new("/tmp");
        let path = socket_path_for(Some(&home), temp);

        assert_eq!(path.parent(), Some(temp));
        let name = path.file_name().unwrap().to_string_lossy().into_owned();
        assert!(
            name.starts_with("crowbar-")
                && Path::new(&name)
                    .extension()
                    .is_some_and(|ext| ext.eq_ignore_ascii_case("sock")),
            "got {name}"
        );
        // Byte-identical to crowbar_client::socket's own derivation, exercised
        // directly so a drift between the two shows up here too.
        let expected = crowbar_client::socket::Location {
            crowbar_home: Some(&home.path),
            temp_dir: temp,
            user_home: None,
        }
        .socket_path()
        .unwrap();
        assert_eq!(path, expected);
    }

    #[test]
    fn a_production_home_sits_under_the_home_directory_itself() {
        let home = CrowbarHome {
            path: PathBuf::from("/Users/someone/.crowbar"),
            overridden: false,
        };
        let path = socket_path_for(Some(&home), Path::new("/tmp"));
        assert_eq!(path, PathBuf::from("/Users/someone/.crowbar/crowbar.sock"),);
    }

    #[test]
    fn no_home_at_all_falls_back_to_the_temp_dir_default() {
        let path = socket_path_for(None, Path::new("/tmp"));
        assert_eq!(path, PathBuf::from("/tmp/crowbar.sock"));
    }

    #[test]
    fn two_distinct_overridden_homes_never_collide() {
        let a = CrowbarHome {
            path: PathBuf::from("/a/.crowbar"),
            overridden: true,
        };
        let b = CrowbarHome {
            path: PathBuf::from("/b/.crowbar"),
            overridden: true,
        };
        let temp = Path::new("/tmp");
        assert_ne!(
            socket_path_for(Some(&a), temp),
            socket_path_for(Some(&b), temp)
        );
    }

    /// The real edge, deterministic because `cfg(test)` implies
    /// `debug_assertions`: this process always resolves a dev-repo-root
    /// override, so `socket_path()` must land in the temp dir with the hashed
    /// name, twice in a row (the daemon and every client must agree on one
    /// path across calls).
    #[test]
    fn socket_path_is_fixed_and_deterministic() {
        let p = socket_path();
        let name = p.file_name().unwrap().to_string_lossy().into_owned();
        assert!(
            name.starts_with("crowbar-")
                && Path::new(&name)
                    .extension()
                    .is_some_and(|ext| ext.eq_ignore_ascii_case("sock")),
            "got {name}"
        );
        assert_eq!(p, socket_path());
    }
}
