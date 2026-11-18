use std::path::{Path, PathBuf};
use std::time::{Duration, SystemTime};

use diagnostics::{Bundle, LogPaths};

use crate::daemon_link::DaemonLink;

const DUMP_TIMEOUT: Duration = Duration::from_secs(10);
const DUMP_LIMIT_BYTES: usize = 8 * 1024 * 1024;
const DOWNLOADS: &str = "Downloads";

const DUMPS: &[(&str, &str)] = &[
    ("daemon-health.json", "/v0/health"),
    ("goroutine-dump.txt", "/debug/pprof/goroutine?debug=2"),
    ("heap-profile.txt", "/debug/pprof/heap?debug=1"),
];

pub(crate) fn destination(
    home: Option<&Path>,
    scratch: &Path,
    is_dir: impl Fn(&Path) -> bool,
) -> PathBuf {
    home.map(|home| home.join(DOWNLOADS))
        .filter(|dir| is_dir(dir))
        .unwrap_or_else(|| scratch.to_path_buf())
}

pub(crate) fn start(
    link: &DaemonLink,
    paths: LogPaths,
    now: SystemTime,
) {
    let socket = link.socket().unwrap_or_default();
    let daemon_log = link.log();
    link.handle().spawn(async move {
        let mut bundle = Bundle::new(env!("CARGO_PKG_VERSION"));
        bundle.add_directory("app", paths.dir());
        add_daemon_log(&mut bundle, daemon_log.as_deref());
        add_dumps(&mut bundle, &socket).await;
        let home = std::env::var_os("HOME").map(PathBuf::from);
        let target = destination(home.as_deref(), &std::env::temp_dir(), Path::is_dir);
        match bundle.write(&target, now) {
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
            supervisor::fetch(socket, target, DUMP_LIMIT_BYTES),
        )
        .await;
        match fetched {
            Ok(Ok(bytes)) => bundle.add_bytes(name, bytes),
            Ok(Err(error)) => bundle.add_missing(name, &error.to_string()),
            Err(elapsed) => bundle.add_missing(name, &format!("timed out: {elapsed}")),
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

    const HOME: &str = "/home/user";
    const SCRATCH: &str = "/scratch";

    struct Case {
        name: &'static str,
        home: Option<&'static str>,
        dirs: &'static [&'static str],
        expected: &'static str,
    }

    const CASES: &[Case] = &[
        Case {
            name: "downloads exists",
            home: Some(HOME),
            dirs: &["/home/user/Downloads"],
            expected: "/home/user/Downloads",
        },
        Case {
            name: "no downloads folder",
            home: Some(HOME),
            dirs: &[],
            expected: SCRATCH,
        },
        Case {
            name: "no home",
            home: None,
            dirs: &["/home/user/Downloads"],
            expected: SCRATCH,
        },
    ];

    #[test]
    fn the_bundle_goes_to_downloads_when_it_exists() {
        for case in CASES {
            let known = |dir: &Path| case.dirs.iter().any(|name| Path::new(name) == dir);
            let found = destination(case.home.map(Path::new), Path::new(SCRATCH), known);
            assert_eq!(found, Path::new(case.expected), "{}", case.name);
        }
    }

    #[test]
    fn every_dump_targets_a_daemon_path() {
        assert!(DUMPS.iter().all(|(_, target)| target.starts_with('/')));
    }
}
