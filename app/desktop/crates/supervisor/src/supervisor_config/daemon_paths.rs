use std::ffi::OsString;
use std::path::{Path, PathBuf};
use std::process::Stdio;
use std::time::Duration;

use serde_json::Value;
use tokio::process::Command;

use crate::error::Error;

const PATHS_TIMEOUT: Duration = Duration::from_secs(10);

/// Where the daemon binds and logs for one Crowbar home, as the daemon binary itself reports
/// them so Go's `core/home` is the only place either is decided.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct DaemonPaths {
    /// The unix socket the daemon listens on.
    pub socket: PathBuf,
    /// The file the daemon's output is captured to.
    pub log: PathBuf,
}

fn absolute(
    report: &Value,
    key: &str,
) -> Result<PathBuf, Error> {
    let text = report
        .get(key)
        .and_then(Value::as_str)
        .ok_or_else(|| Error::Paths(format!("paths output has no {key}")))?;
    let path = PathBuf::from(text);
    if path.is_absolute() {
        return Ok(path);
    }
    Err(Error::Paths(format!(
        "paths output: {key} is not absolute: {text:?}"
    )))
}

impl DaemonPaths {
    /// Reads the output of `crowbar-api paths`: `{"home": ..., "socket": ..., "log": ...}`.
    ///
    /// # Errors
    /// [`Error::Paths`] for output that is not JSON or names a missing or relative path.
    pub fn parse(stdout: &[u8]) -> Result<Self, Error> {
        let report: Value = serde_json::from_slice(stdout)
            .map_err(|e| Error::Paths(format!("paths output is not JSON: {e}")))?;
        Ok(Self {
            socket: absolute(&report, "socket")?,
            log: absolute(&report, "log")?,
        })
    }

    /// Runs `<binary> paths`, giving up after ten seconds, with `CROWBAR_HOME` set to `home` when one is given.
    ///
    /// # Errors
    /// [`Error::Spawn`] when the binary cannot run, [`Error::Paths`] when it fails, times out or
    /// prints something unusable.
    pub async fn resolve(
        binary: &Path,
        home: Option<&Path>,
    ) -> Result<Self, Error> {
        Self::resolve_within(binary, home, PATHS_TIMEOUT).await
    }

    async fn resolve_within(
        binary: &Path,
        home: Option<&Path>,
        limit: Duration,
    ) -> Result<Self, Error> {
        let mut command = Command::new(binary);
        command.arg("paths").stdin(Stdio::null()).kill_on_drop(true);
        if let Some(home) = home {
            command.env("CROWBAR_HOME", home);
        }
        let finished = tokio::time::timeout(limit, command.output())
            .await
            .map_err(Error::PathsTimeout)?
            .map_err(|source| Error::Spawn {
                path: binary.to_path_buf(),
                source,
            })?;
        if !finished.status.success() {
            let stderr = String::from_utf8_lossy(&finished.stderr);
            return Err(Error::Paths(stderr.trim().to_owned()));
        }
        Self::parse(&finished.stdout)
    }
}

/// The `CROWBAR_HOME` to hand the daemon: the environment's own when it names one, else
/// `fallback` (a development checkout's `.crowbar`). An empty value counts as none, as in Go.
#[must_use]
pub fn child_home(
    current: Option<OsString>,
    fallback: Option<PathBuf>,
) -> Option<PathBuf> {
    match current {
        Some(home) if !home.is_empty() => Some(PathBuf::from(home)),
        _ => fallback,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn parse_reads_socket_and_log() {
        let out = br#"{"home":"/Users/a/.crowbar","socket":"/tmp/crowbar-1f.sock","log":"/Users/a/.crowbar/logs/daemon.log"}"#;
        let paths = DaemonPaths::parse(out).unwrap();
        assert_eq!(paths.socket, PathBuf::from("/tmp/crowbar-1f.sock"));
        assert_eq!(
            paths.log,
            PathBuf::from("/Users/a/.crowbar/logs/daemon.log")
        );
    }

    #[test]
    fn parse_rejects_relative_or_missing_fields() {
        for out in [
            r#"{"home":"/h","socket":"crowbar.sock","log":"/l"}"#,
            r#"{"home":"/h","socket":"/s.sock","log":"daemon.log"}"#,
            r#"{"home":"/h","log":"/l"}"#,
            r#"{"home":"/h","socket":"/s.sock"}"#,
            r#"{"home":"/h","socket":"/s.sock","log":""}"#,
            r#"{"home":"/h","socket":7,"log":"/l"}"#,
            "starting daemon...",
            "",
        ] {
            assert!(DaemonPaths::parse(out.as_bytes()).is_err(), "{out}");
        }
    }

    #[test]
    fn a_set_home_is_not_replaced_and_an_empty_one_counts_as_unset() {
        let dev = Some(PathBuf::from("/work/.crowbar"));
        let cases = [
            (
                "set",
                Some("/mine".into()),
                dev.clone(),
                Some(PathBuf::from("/mine")),
            ),
            ("empty", Some(OsString::new()), dev.clone(), dev.clone()),
            ("unset", None, dev.clone(), dev),
            ("unset without fallback", None, None, None),
        ];
        for (name, current, fallback, expected) in cases {
            assert_eq!(child_home(current, fallback), expected, "{name}");
        }
    }

    #[tokio::test]
    async fn a_binary_that_cannot_run_is_a_spawn_error() {
        let err = DaemonPaths::resolve(Path::new("/definitely/not/crowbar-api"), None)
            .await
            .unwrap_err();
        assert!(matches!(err, Error::Spawn { .. }), "{err}");
    }

    #[tokio::test]
    #[ignore = "real-infra: runs a real script that never answers to prove the timeout"]
    async fn a_binary_that_never_answers_is_given_up_on() {
        use std::os::unix::fs::PermissionsExt;
        let dir = tempfile::tempdir().unwrap();
        let script = dir.path().join("hang.sh");
        std::fs::write(&script, "#!/bin/sh\nexec sleep 60\n").unwrap();
        std::fs::set_permissions(&script, std::fs::Permissions::from_mode(0o755)).unwrap();
        let err = DaemonPaths::resolve_within(&script, None, Duration::from_millis(200))
            .await
            .unwrap_err();
        assert!(matches!(err, Error::PathsTimeout(_)), "{err}");
    }
}
