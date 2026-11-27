use std::path::{Path, PathBuf};

const LOG_DIR: &str = "logs";
const LOG_FILE: &str = "crowbar.log";

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct LogPaths {
    dir: PathBuf,
}

impl LogPaths {
    #[must_use]
    pub fn in_data_dir(data_dir: &Path) -> Self {
        Self {
            dir: data_dir.join(LOG_DIR),
        }
    }

    #[must_use]
    pub fn dir(&self) -> &Path {
        &self.dir
    }

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
