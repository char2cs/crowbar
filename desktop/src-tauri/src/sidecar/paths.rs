//! The Crowbar home and daemon socket, as the daemon binary reports them, so
//! Go's `core/home` is the only place either is decided.

use std::error::Error;
use std::ffi::OsStr;
use std::path::PathBuf;
use std::sync::OnceLock;

use tauri::{AppHandle, Runtime};
use tauri_plugin_shell::{process::Command, ShellExt};

#[derive(Debug)]
pub struct ResolvedPaths {
    pub home: PathBuf,
    pub socket: PathBuf,
}

static PATHS: OnceLock<ResolvedPaths> = OnceLock::new();

/// Reads `crowbar-api paths`, which prints `{"home": ..., "socket": ...}`.
pub fn parse_paths(stdout: &[u8]) -> Result<ResolvedPaths, String> {
    let report: serde_json::Value =
        serde_json::from_slice(stdout).map_err(|e| format!("paths output is not JSON: {e}"))?;
    let absolute = |key: &str| {
        let text = report
            .get(key)
            .and_then(serde_json::Value::as_str)
            .ok_or_else(|| format!("paths output has no {key}"))?;
        let path = PathBuf::from(text);
        if path.is_absolute() {
            Ok(path)
        } else {
            Err(format!("paths output: {key} is not absolute: {text:?}"))
        }
    };
    Ok(ResolvedPaths {
        home: absolute("home")?,
        socket: absolute("socket")?,
    })
}

/// A debug build roots all state in the checkout being developed, so a dev
/// instance never shares a home or socket with the production app.
#[cfg(debug_assertions)]
pub fn dev_home_override(repo_root: &std::path::Path) -> PathBuf {
    repo_root.join(".crowbar")
}

/// The `CROWBAR_HOME` to hand the daemon binary: only a debug build whose
/// environment names none, an empty value counting as none as it does in Go.
fn injected_home(current: Option<&OsStr>) -> Option<PathBuf> {
    if current.is_some_and(|home| !home.is_empty()) {
        return None;
    }
    #[cfg(debug_assertions)]
    return std::path::Path::new(env!("CARGO_MANIFEST_DIR"))
        .parent()
        .and_then(std::path::Path::parent)
        .map(dev_home_override);
    #[cfg(not(debug_assertions))]
    None
}

/// Gives `cmd` the dev home when this build owes it one. `resolve` and `spawn`
/// both go through here, so `paths` and `serve` always agree on the home.
pub(super) fn with_dev_home(cmd: Command) -> Command {
    match injected_home(std::env::var_os("CROWBAR_HOME").as_deref()) {
        Some(home) => cmd.env("CROWBAR_HOME", home.to_string_lossy().to_string()),
        None => cmd,
    }
}

/// Asks the daemon binary where the home and socket are and caches the answer.
/// Runs once in `setup`, before any window can reach the daemon.
pub async fn resolve<R: Runtime>(app: &AppHandle<R>) -> Result<(), Box<dyn Error + Send + Sync>> {
    let output = with_dev_home(app.shell().sidecar("crowbar-api")?.args(["paths"]))
        .output()
        .await?;
    if !output.status.success() {
        return Err(String::from_utf8_lossy(&output.stderr)
            .trim()
            .to_string()
            .into());
    }
    let paths = parse_paths(&output.stdout)?;
    PATHS.get_or_init(|| paths);
    Ok(())
}

fn resolved() -> &'static ResolvedPaths {
    PATHS
        .get()
        .expect("sidecar::resolve runs in setup, before any window can reach the daemon")
}

pub fn socket_path() -> PathBuf {
    resolved().socket.clone()
}

/// The daemon's captured stdout/stderr: `<crowbar home>/logs/daemon.log`.
/// This is the only place a Go panic's stack trace survives in the packaged
/// app — the sidecar's pipes are otherwise connected to nothing.
pub fn daemon_log_path() -> PathBuf {
    resolved().home.join("logs").join("daemon.log")
}

#[cfg(test)]
mod tests {
    use super::parse_paths;
    use std::path::PathBuf;

    #[test]
    fn parse_paths_reads_home_and_socket() {
        let out = br#"{"home":"/Users/a/.crowbar","socket":"/tmp/crowbar-1f.sock"}"#;
        let paths = parse_paths(out).unwrap();
        assert_eq!(paths.home, PathBuf::from("/Users/a/.crowbar"));
        assert_eq!(paths.socket, PathBuf::from("/tmp/crowbar-1f.sock"));
    }

    #[test]
    fn parse_paths_rejects_relative_or_missing_fields() {
        for (why, out) in [
            (
                "relative socket",
                r#"{"home":"/h","socket":"crowbar.sock"}"#,
            ),
            ("relative home", r#"{"home":"h","socket":"/s.sock"}"#),
            ("missing home", r#"{"socket":"/s.sock"}"#),
            ("missing socket", r#"{"home":"/h"}"#),
            ("empty home", r#"{"home":"","socket":"/s.sock"}"#),
            ("not a string", r#"{"home":7,"socket":"/s.sock"}"#),
            ("not json", "starting daemon..."),
            ("empty output", ""),
        ] {
            assert!(
                parse_paths(out.as_bytes()).is_err(),
                "{why} must be refused"
            );
        }
    }

    #[cfg(debug_assertions)]
    mod dev_home {
        use super::super::{dev_home_override, injected_home};
        use std::ffi::OsStr;
        use std::path::{Path, PathBuf};

        #[test]
        fn dev_home_override_is_dot_crowbar_in_the_repo_root() {
            assert_eq!(
                dev_home_override(Path::new("/work/tree")),
                PathBuf::from("/work/tree/.crowbar")
            );
        }

        #[test]
        fn a_set_crowbar_home_is_not_replaced_by_the_dev_override() {
            assert_eq!(injected_home(Some(OsStr::new("/mine"))), None);
            let unset =
                injected_home(None).expect("a debug build with no CROWBAR_HOME injects one");
            assert!(unset.ends_with(".crowbar"), "got {}", unset.display());
            assert_eq!(
                injected_home(Some(OsStr::new(""))),
                Some(unset),
                "the daemon treats an empty CROWBAR_HOME as unset, so the shell must too"
            );
        }
    }
}
