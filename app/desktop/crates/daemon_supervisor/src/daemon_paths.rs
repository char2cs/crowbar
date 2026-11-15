use std::ffi::OsString;
use std::path::{Path, PathBuf};

use serde_json::Value;
use tokio::process::Command;

use crate::error::Error;

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

    /// Runs `<binary> paths` with `CROWBAR_HOME` set to `home` when one is given.
    ///
    /// # Errors
    /// [`Error::Spawn`] when the binary cannot run, [`Error::Paths`] when it fails or prints
    /// something unusable.
    pub async fn resolve(
        binary: &Path,
        home: Option<&Path>,
    ) -> Result<Self, Error> {
        let mut command = Command::new(binary);
        command.arg("paths");
        if let Some(home) = home {
            command.env("CROWBAR_HOME", home);
        }
        let output = command.output().await.map_err(|source| Error::Spawn {
            path: binary.to_path_buf(),
            source,
        })?;
        if !output.status.success() {
            let stderr = String::from_utf8_lossy(&output.stderr);
            return Err(Error::Paths(stderr.trim().to_owned()));
        }
        Self::parse(&output.stdout)
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
        assert_eq!(
            child_home(Some("/mine".into()), dev.clone()),
            Some(PathBuf::from("/mine"))
        );
        assert_eq!(child_home(Some(OsString::new()), dev.clone()), dev);
        assert_eq!(child_home(None, dev.clone()), dev);
        assert_eq!(child_home(None, None), None);
    }

    #[tokio::test]
    async fn a_binary_that_cannot_run_is_a_spawn_error() {
        let err = DaemonPaths::resolve(Path::new("/definitely/not/crowbar-api"), None)
            .await
            .unwrap_err();
        assert!(matches!(err, Error::Spawn { .. }), "{err}");
    }
}
