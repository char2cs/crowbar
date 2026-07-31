//! Where the daemon's control socket lives.
//!
//! This is a *contract*, not a convenience: the Go daemon binds the path and
//! every client dials it, so a derivation that differs by one byte produces a
//! bare "connection refused" with nothing pointing at the cause. The
//! authoritative implementation is `overrideSocketPath` in
//! `api/internal/core/gateway/transports/socket.go`; the Tauri shell carries a
//! second copy in `desktop/src-tauri/src/sidecar/mod.rs`. This is the third,
//! and it exists because §4.2 forbids `crowbar-client` from depending on
//! either.
//!
//! The whole module is pure — no filesystem, no environment, no clock — so the
//! agreement with Go is a unit-testable property rather than something you find
//! out at runtime. [`RawEnv::from_env`] is the one impure edge, and it does
//! nothing but read three variables and hand them here.

use std::ffi::OsString;
use std::fmt;
use std::path::{Path, PathBuf};

/// The name the daemon binds under a non-overridden home.
const DEFAULT_SOCKET_NAME: &str = "crowbar.sock";

/// The directory a non-overridden home lives in, under the user's home.
const HOME_DIR_NAME: &str = ".crowbar";

/// FNV-1a 64-bit offset basis.
const FNV_OFFSET_BASIS: u64 = 0xcbf2_9ce4_8422_2325;

/// FNV-1a 64-bit prime.
const FNV_PRIME: u64 = 0x0000_0100_0000_01b3;

/// The three environment values the socket path is a function of.
///
/// Held as borrowed paths so the derivation can be exercised against inputs
/// that do not exist on the machine running the test.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct Location<'a> {
    /// `$CROWBAR_HOME`. `None`, or a path whose string form is empty, means
    /// unset — Go tests `override != ""` and the Tauri shell tests
    /// `!dir.is_empty()`, so an exported-but-empty variable is *not* an
    /// override in either.
    pub crowbar_home: Option<&'a Path>,
    /// `$TMPDIR`, or the platform default. Only consulted for an overridden
    /// home.
    pub temp_dir: &'a Path,
    /// `$HOME`. Only consulted when there is no override.
    pub user_home: Option<&'a Path>,
}

/// The environment did not name a home the socket path could be built from.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct NoHome;

impl fmt::Display for NoHome {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        f.write_str(
            "neither CROWBAR_HOME nor HOME is set, so the daemon socket path cannot be derived",
        )
    }
}

impl std::error::Error for NoHome {}

impl<'a> Location<'a> {
    /// The path the daemon binds and every client dials.
    ///
    /// With `CROWBAR_HOME` set: `$TMPDIR/crowbar-<fnv1a64(home)>.sock`. The
    /// socket cannot live *inside* an overridden home — macOS caps `sun_path`
    /// at 104 bytes and Crowbar's workspace worktree paths routinely exceed
    /// that — so the home is hashed down to a short name in the temp
    /// directory, which also keeps concurrent dev instances apart.
    ///
    /// Without it: `$HOME/.crowbar/crowbar.sock`.
    ///
    /// # Errors
    ///
    /// [`NoHome`] when neither `CROWBAR_HOME` nor `HOME` is set. Unlike Go's
    /// `os.UserHomeDir`, there is no fallback to guess from — and silently
    /// dialling some other path would be worse than saying so.
    pub fn socket_path(&self) -> Result<PathBuf, NoHome> {
        match self.override_home() {
            Some(home) => Ok(self.temp_dir.join(override_socket_name(home))),
            None => self
                .user_home
                .map(|home| home.join(HOME_DIR_NAME).join(DEFAULT_SOCKET_NAME))
                .ok_or(NoHome),
        }
    }

    /// `crowbar_home` if it is set to something non-empty.
    fn override_home(&self) -> Option<&'a Path> {
        self.crowbar_home
            .filter(|home| !home.as_os_str().is_empty())
    }
}

/// An owned snapshot of the process environment, since `std` hands back
/// `OsString` and [`Location`] borrows.
///
/// Kept separate from [`Location`] so the derivation itself never touches the
/// process environment: a test can build any `Location` it likes without
/// mutating global state, which is the only way to test this concurrently.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct RawEnv {
    crowbar_home: Option<OsString>,
    temp_dir: PathBuf,
    user_home: Option<OsString>,
}

impl RawEnv {
    /// Reads the three variables the derivation depends on.
    ///
    /// This is the module's only impure function, and it makes no decisions —
    /// everything with a branch in it lives in [`Location::socket_path`].
    #[must_use]
    pub fn from_env() -> Self {
        Self {
            crowbar_home: std::env::var_os("CROWBAR_HOME"),
            temp_dir: std::env::temp_dir(),
            user_home: std::env::var_os("HOME"),
        }
    }

    /// Borrows this snapshot as the pure input to [`Location::socket_path`].
    #[must_use]
    pub fn location(&self) -> Location<'_> {
        Location {
            crowbar_home: self.crowbar_home.as_deref().map(Path::new),
            temp_dir: &self.temp_dir,
            user_home: self.user_home.as_deref().map(Path::new),
        }
    }
}

/// The socket filename for an overridden home: `crowbar-<fnv1a64(home)>.sock`.
///
/// **The format string is `{:x}`, never `{:016x}`.** Go's `%x` does not
/// zero-pad, so a hash whose top nibble is zero yields a *fifteen*-character
/// hex string and a filename one byte shorter than the usual one. Padding it
/// looks correct, reviews clean, and then fails to find the daemon for roughly
/// one home in sixteen. `socket_name_is_not_zero_padded` below pins a real
/// input that exhibits it.
///
/// The hash input is the home path's string form exactly as it appears in
/// `CROWBAR_HOME` — not canonicalised, not trailing-slash-normalised. Go hashes
/// the raw environment string, so anything else here would silently disagree.
fn override_socket_name(home: &Path) -> String {
    let hash = fnv1a64(home.to_string_lossy().as_bytes());
    format!("crowbar-{hash:x}.sock")
}

/// FNV-1a, 64-bit, byte for byte what Go's `hash/fnv.New64a` produces.
#[must_use]
pub fn fnv1a64(bytes: &[u8]) -> u64 {
    let mut hash = FNV_OFFSET_BASIS;
    for byte in bytes {
        hash ^= u64::from(*byte);
        hash = hash.wrapping_mul(FNV_PRIME);
    }
    hash
}

#[cfg(test)]
mod tests {
    use super::*;

    /// `CROWBAR_HOME` of the `rewrite/rust` worktree used for item 0.4's
    /// harness, and the socket a live Go daemon was observed bound to. This is
    /// the cross-implementation check: the expectation was produced by the Go
    /// side, not by this code.
    const HARNESS_HOME: &str = "/Users/char2cs/.crowbar/projects/cb81ea03-54b7-4093-8bdd-fdd6b183e91d/github.com/char2cs/crowbar/rewrite/rust/worktree/.crowbar";
    const HARNESS_HASH: &str = "6d4f21ce150add3c";

    fn at(crowbar_home: Option<&'static str>, user_home: Option<&'static str>) -> PathBuf {
        let temp = Path::new("/tmp");
        Location {
            crowbar_home: crowbar_home.map(Path::new),
            temp_dir: temp,
            user_home: user_home.map(Path::new),
        }
        .socket_path()
        .expect("a home was supplied")
    }

    #[test]
    fn fnv1a64_matches_go_reference_vectors() {
        // The canonical FNV-1a 64 test vectors, as Go's hash/fnv produces them.
        assert_eq!(fnv1a64(b""), 0xcbf2_9ce4_8422_2325);
        assert_eq!(fnv1a64(b"a"), 0xaf63_dc4c_8601_ec8c);
        assert_eq!(fnv1a64(b"foobar"), 0x8594_4171_f739_67e8);
    }

    #[test]
    fn override_home_hashes_to_the_path_the_go_daemon_bound() {
        assert_eq!(
            at(Some(HARNESS_HOME), None),
            PathBuf::from(format!("/tmp/crowbar-{HARNESS_HASH}.sock")),
        );
    }

    /// The trap this module exists for.
    ///
    /// `fnv1a64("/home/10/.crowbar")` is `0x0bbb5af7c35603fa` — a leading zero
    /// nibble. Go's `fmt.Sprintf("crowbar-%x.sock", h)` emits fifteen hex
    /// digits for it. `{:016x}` would emit sixteen, the client would dial a
    /// path no daemon ever bound, and the failure would surface as a bare
    /// connection error. Roughly one home in sixteen is affected.
    #[test]
    fn socket_name_is_not_zero_padded() {
        let home = Path::new("/home/10/.crowbar");

        assert_eq!(fnv1a64(b"/home/10/.crowbar"), 0x0bbb_5af7_c356_03fa);
        assert_eq!(override_socket_name(home), "crowbar-bbb5af7c35603fa.sock");
        assert_ne!(override_socket_name(home), "crowbar-0bbb5af7c35603fa.sock");

        // Stated as the invariant rather than as one example: the hex run
        // between the prefix and the extension is however many digits the
        // number needs, and here that is fifteen, not sixteen.
        let name = override_socket_name(home);
        let hex = name
            .trim_start_matches("crowbar-")
            .trim_end_matches(".sock");
        assert_eq!(hex.len(), 15);
    }

    #[test]
    fn without_an_override_the_socket_sits_under_the_user_home() {
        assert_eq!(
            at(None, Some("/Users/someone")),
            PathBuf::from("/Users/someone/.crowbar/crowbar.sock"),
        );
    }

    /// Go reads `os.Getenv`, which cannot distinguish unset from empty, and
    /// treats `""` as "not overridden". Both this and the Tauri shell must
    /// agree, or an exported-but-empty `CROWBAR_HOME` sends the two apps to
    /// different sockets.
    #[test]
    fn an_empty_override_is_not_an_override() {
        assert_eq!(
            at(Some(""), Some("/Users/someone")),
            PathBuf::from("/Users/someone/.crowbar/crowbar.sock"),
        );
    }

    #[test]
    fn an_override_wins_over_the_user_home() {
        let overridden = at(Some(HARNESS_HOME), Some("/Users/someone"));
        assert_eq!(
            overridden,
            PathBuf::from(format!("/tmp/crowbar-{HARNESS_HASH}.sock")),
        );
    }

    #[test]
    fn the_temp_dir_is_used_verbatim_for_an_override() {
        let temp = Path::new("/var/folders/3k/T");
        let path = Location {
            crowbar_home: Some(Path::new(HARNESS_HOME)),
            temp_dir: temp,
            user_home: None,
        }
        .socket_path()
        .expect("an override was supplied");
        assert_eq!(path.parent(), Some(temp));
    }

    /// Two different homes must never collide onto one socket — that is the
    /// entire reason the path is hashed rather than fixed.
    #[test]
    fn distinct_homes_get_distinct_sockets() {
        assert_ne!(at(Some("/a/.crowbar"), None), at(Some("/b/.crowbar"), None));
    }

    #[test]
    fn no_home_at_all_is_an_error_not_a_guess() {
        let err = Location {
            crowbar_home: None,
            temp_dir: Path::new("/tmp"),
            user_home: None,
        }
        .socket_path()
        .expect_err("nothing to derive from");
        assert_eq!(err, NoHome);
        assert!(err.to_string().contains("CROWBAR_HOME"));
        // Debug is what a `?` propagation prints; keep it non-empty.
        assert!(!format!("{err:?}").is_empty());
    }

    /// `from_env` is the impure edge; `location` is the borrow that feeds the
    /// pure half. Asserting the round trip keeps the two from drifting without
    /// this test having to mutate the process environment.
    #[test]
    fn read_env_snapshots_borrow_as_a_location() {
        let raw = RawEnv::from_env();
        let loc = raw.location();
        assert_eq!(loc.temp_dir, std::env::temp_dir());
        assert_eq!(
            loc.crowbar_home.map(Path::to_path_buf),
            std::env::var_os("CROWBAR_HOME").map(PathBuf::from),
        );
        assert_eq!(
            loc.user_home.map(Path::to_path_buf),
            std::env::var_os("HOME").map(PathBuf::from),
        );
        // A snapshot taken twice in the same process is the same snapshot.
        assert_eq!(raw, RawEnv::from_env());
        assert!(!format!("{raw:?}").is_empty());
        assert_eq!(loc, raw.location());
        assert!(!format!("{loc:?}").is_empty());
    }
}
