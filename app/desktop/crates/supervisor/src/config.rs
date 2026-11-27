mod daemon_paths;
mod locate_binary;
mod timings;

pub use daemon_paths::DaemonPaths;
pub use locate_binary::locate_binary;
pub use timings::Timings;

use std::path::PathBuf;

#[derive(Debug, Clone)]
pub struct Config {
    pub binary: PathBuf,
    pub paths: DaemonPaths,
    pub home: Option<PathBuf>,
    pub version: String,
    pub timings: Timings,
}

impl Config {
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

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_new_config_runs_with_production_timings() {
        let paths = DaemonPaths {
            socket: PathBuf::from("/s.sock"),
            log: PathBuf::from("/l.log"),
        };
        let config = Config::new(PathBuf::from("/bin/crowbar-api"), paths, None, "1.2.3");
        assert_eq!(config.timings.restart_max, Timings::default().restart_max);
    }
}
