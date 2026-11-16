use std::path::{Path, PathBuf};
use std::time::{Duration, SystemTime};

use shell_diagnostics::{Bundle, LogPaths};

use crate::daemon_link::DaemonLink;

const DUMP_TIMEOUT: Duration = Duration::from_secs(10);
const DUMP_LIMIT_BYTES: usize = 8 * 1024 * 1024;
const DOWNLOADS: &str = "Downloads";

const DUMPS: &[(&str, &str)] = &[
    ("daemon-health.json", "/v0/health"),
    ("goroutine-dump.txt", "/debug/pprof/goroutine?debug=2"),
    ("heap-profile.txt", "/debug/pprof/heap?debug=1"),
];

pub(crate) fn destination(home: Option<&Path>) -> PathBuf {
    home.map(|home| home.join(DOWNLOADS))
        .filter(|dir| dir.is_dir())
        .unwrap_or_else(std::env::temp_dir)
}

pub(crate) fn start(
    link: &DaemonLink,
    paths: LogPaths,
) {
    let socket = link.socket().unwrap_or_default();
    let daemon_log = link.log();
    link.handle().spawn(async move {
        let mut bundle = Bundle::new(env!("CARGO_PKG_VERSION"));
        bundle.add_directory("app", paths.dir());
        add_daemon_log(&mut bundle, daemon_log.as_deref());
        add_dumps(&mut bundle, &socket).await;
        let home = std::env::var_os("HOME").map(PathBuf::from);
        match bundle.write(&destination(home.as_deref()), SystemTime::now()) {
            Ok(path) => reveal(&path),
            Err(error) => tracing::error!(%error, "the diagnostics bundle was not written"),
        }
    });
}

fn add_daemon_log(
    bundle: &mut Bundle,
    log: Option<&Path>,
) {
    let Some(log) = log else {
        bundle.add_missing("daemon.log", "the daemon was never started");
        return;
    };
    bundle.add_file("daemon.log", log);
    let mut rotated = log.as_os_str().to_owned();
    rotated.push(".1");
    bundle.add_file("daemon.log.1", Path::new(&rotated));
}

async fn add_dumps(
    bundle: &mut Bundle,
    socket: &Path,
) {
    for (name, target) in DUMPS {
        let fetched = tokio::time::timeout(
            DUMP_TIMEOUT,
            daemon_supervisor::fetch(socket, target, DUMP_LIMIT_BYTES),
        )
        .await;
        match fetched {
            Ok(Ok(bytes)) => bundle.add_bytes(name, bytes),
            Ok(Err(error)) => bundle.add_missing(name, &error.to_string()),
            Err(_) => bundle.add_missing(name, "timed out"),
        }
    }
}

fn reveal(path: &Path) {
    if let Err(error) = shell_platform::reveal_in_file_manager(path) {
        tracing::warn!(%error, "the diagnostics bundle could not be shown");
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_bundle_goes_to_downloads_when_it_exists() {
        let home = tempfile::tempdir().unwrap();
        assert_eq!(destination(Some(home.path())), std::env::temp_dir());
        std::fs::create_dir(home.path().join("Downloads")).unwrap();
        assert_eq!(
            destination(Some(home.path())),
            home.path().join("Downloads")
        );
        assert_eq!(destination(None), std::env::temp_dir());
    }

    #[test]
    fn every_dump_targets_a_daemon_path() {
        assert!(DUMPS.iter().all(|(_, target)| target.starts_with('/')));
    }
}
