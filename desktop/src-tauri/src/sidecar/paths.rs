//! The daemon socket and daemon log, as the daemon binary reports them for its
//! home, so Go's `core/home` is the only place either is decided.

use std::error::Error;
use std::ffi::OsStr;
use std::path::PathBuf;
use std::sync::OnceLock;

use tauri::{AppHandle, Manager, Runtime};
use tauri_plugin_dialog::{DialogExt, MessageDialogKind};
use tauri_plugin_shell::{process::Command, ShellExt};

#[derive(Debug)]
pub struct ResolvedPaths {
    pub socket: PathBuf,
    pub log: PathBuf,
}

static PATHS: OnceLock<ResolvedPaths> = OnceLock::new();

/// Reads `crowbar-api paths`, which prints `{"home": ..., "socket": ..., "log": ...}`;
/// the shell needs only the socket and the log.
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
        socket: absolute("socket")?,
        log: absolute("log")?,
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

/// Resolves the paths in `setup`, before any window can reach the daemon, or
/// tells the user why Crowbar cannot start and quits once they dismiss it. The
/// dialog never blocks: setup runs on the main thread, which it draws on.
pub fn resolve_or_explain<R: Runtime>(app: &AppHandle<R>) -> bool {
    let outcome = tauri::async_runtime::block_on(resolve(app));
    explain_failure(outcome, |text| {
        for window in app.webview_windows().into_values() {
            if let Err(e) = window.hide() {
                log::warn!("cannot hide window {}: {e}", window.label());
            }
        }
        let quit = app.clone();
        app.dialog()
            .message(text)
            .kind(MessageDialogKind::Error)
            .title("Crowbar cannot start")
            .show(move |_| quit.exit(1));
    })
}

/// Whether setup may go on after `outcome`; a failure is handed to `explain`
/// as the text the user reads.
fn explain_failure(
    outcome: Result<(), Box<dyn Error + Send + Sync>>,
    explain: impl FnOnce(String),
) -> bool {
    let Err(e) = outcome else {
        return true;
    };
    log::error!("cannot resolve the Crowbar home: {e}");
    explain(format!(
        "Crowbar could not find where its data lives, so it cannot start.\n\n{e}"
    ));
    false
}

/// Asks the daemon binary where the socket and log are and caches the answer.
async fn resolve<R: Runtime>(app: &AppHandle<R>) -> Result<(), Box<dyn Error + Send + Sync>> {
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

/// The daemon's captured stdout/stderr, where `paths` says it goes. This is
/// the only place a Go panic's stack trace survives in the packaged app — the
/// sidecar's pipes are otherwise connected to nothing.
pub fn daemon_log_path() -> PathBuf {
    resolved().log.clone()
}

#[cfg(test)]
mod tests {
    use super::{explain_failure, parse_paths};
    use std::path::PathBuf;

    #[test]
    fn a_failed_resolve_is_explained_and_stops_setup_instead_of_panicking() {
        let mut told = None;
        let go_on = explain_failure(
            Err("home: CROWBAR_HOME must be an absolute path".into()),
            |text| told = Some(text),
        );
        assert!(
            !go_on,
            "setup must not go on to spawn a daemon it cannot find"
        );
        let told = told.expect("the user is told why Crowbar cannot start");
        assert!(
            told.contains("CROWBAR_HOME must be an absolute path"),
            "got {told:?}"
        );
    }

    #[test]
    fn a_resolved_home_lets_setup_go_on_and_tells_nobody() {
        assert!(explain_failure(Ok(()), |text| panic!("told {text:?}")));
    }

    #[test]
    fn parse_paths_reads_socket_and_log() {
        let out = br#"{"home":"/Users/a/.crowbar","socket":"/tmp/crowbar-1f.sock","log":"/Users/a/.crowbar/logs/daemon.log"}"#;
        let paths = parse_paths(out).unwrap();
        assert_eq!(paths.socket, PathBuf::from("/tmp/crowbar-1f.sock"));
        assert_eq!(
            paths.log,
            PathBuf::from("/Users/a/.crowbar/logs/daemon.log")
        );
    }

    #[test]
    fn parse_paths_rejects_relative_or_missing_fields() {
        for (why, out) in [
            (
                "relative socket",
                r#"{"home":"/h","socket":"crowbar.sock","log":"/l"}"#,
            ),
            (
                "relative log",
                r#"{"home":"/h","socket":"/s.sock","log":"daemon.log"}"#,
            ),
            ("missing socket", r#"{"home":"/h","log":"/l"}"#),
            ("missing log", r#"{"home":"/h","socket":"/s.sock"}"#),
            ("empty log", r#"{"home":"/h","socket":"/s.sock","log":""}"#),
            ("not a string", r#"{"home":"/h","socket":7,"log":"/l"}"#),
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
