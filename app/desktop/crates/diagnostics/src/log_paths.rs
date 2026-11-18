use std::path::{Path, PathBuf};

const LOG_DIR: &str = "logs";
const LOG_FILE: &str = "crowbar.log";

/// Where the shell keeps its log and crash reports, inside the shell data directory.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct LogPaths {
    dir: PathBuf,
}

impl LogPaths {
    /// The paths under `data_dir`.
    #[must_use]
    pub fn in_data_dir(data_dir: &Path) -> Self {
        Self {
            dir: data_dir.join(LOG_DIR),
        }
    }

    /// The directory holding the log, its older generations and crash reports.
    #[must_use]
    pub fn dir(&self) -> &Path {
        &self.dir
    }

    /// The log file being written; the "Open log" target.
    #[must_use]
    pub fn current(&self) -> PathBuf {
        self.dir.join(LOG_FILE)
    }
}

#[cfg(test)]
mod tests {
    use std::path::Path;

    use super::*;

    #[test]
    fn the_log_lives_under_the_data_dir() {
        let paths = LogPaths::in_data_dir(Path::new("/data/crowbar"));
        assert_eq!(paths.dir(), Path::new("/data/crowbar/logs"));
        assert_eq!(paths.current(), Path::new("/data/crowbar/logs/crowbar.log"));
    }
}
