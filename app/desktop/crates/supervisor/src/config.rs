mod daemon_paths;
mod locate_binary;
mod timings;

pub use daemon_paths::DaemonPaths;
pub use locate_binary::locate_binary;
pub use timings::Timings;

use std::path::PathBuf;

/// What the supervisor needs to run one daemon.
#[derive(Debug, Clone)]
pub struct Config {
    /// The `crowbar-api` binary.
    pub binary: PathBuf,
    /// Where the daemon binds and logs.
    pub paths: DaemonPaths,
    /// The `CROWBAR_HOME` to give the daemon, so it roots its state where the shell expects its
    /// socket; `None` leaves the daemon's own environment alone.
    pub home: Option<PathBuf>,
    /// The app version, printed in the crash report.
    pub version: String,
    /// Timings and limits.
    pub timings: Timings,
}

impl Config {
    /// A config with production timings.
    #[must_use]
    pub fn new(
        binary: PathBuf,
        paths: DaemonPaths,
        home: Option<PathBuf>,
        version: impl Into<String>,
    ) -> Self {
        Self {
            binary,
            paths,
            home,
            version: version.into(),
            timings: Timings::default(),
        }
    }
}
