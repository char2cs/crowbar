use std::fs::{File, OpenOptions};
use std::io::Write;
use std::path::{Path, PathBuf};
use std::sync::{Mutex, MutexGuard, PoisonError};

use crate::error::Error;

const ROTATE_CHECK_EVERY: u32 = 5000;

/// The daemon's captured stdout and stderr, one line per entry, rotated to `.1` at a size cap.
/// It is the only place a Go panic's stack trace survives in the packaged app.
#[derive(Debug)]
pub struct DaemonLog {
    path: PathBuf,
    max_bytes: u64,
    state: Mutex<(File, u32)>,
}

impl DaemonLog {
    /// Opens `path` for appending, creating its directory and rotating a full previous log.
    ///
    /// # Errors
    /// [`Error::Log`] when the file cannot be opened.
    pub fn open(path: &Path, max_bytes: u64) -> Result<Self, Error> {
        let log_error = |source| Error::Log {
            path: path.to_path_buf(),
            source,
        };
        if let Some(dir) = path.parent() {
            std::fs::create_dir_all(dir).map_err(log_error)?;
        }
        rotate_if_needed(path, max_bytes).map_err(log_error)?;
        let file = open_append(path).map_err(log_error)?;
        Ok(Self {
            path: path.to_path_buf(),
            max_bytes,
            state: Mutex::new((file, 0)),
        })
    }

    /// Appends `prefix text` as one line; a log that cannot be written is dropped, never fatal.
    pub fn line(&self, prefix: &str, text: &str) {
        let mut state = self.lock();
        state.1 += 1;
        if state.1 >= ROTATE_CHECK_EVERY {
            state.1 = 0;
            self.rotate(&mut state);
        }
        let trimmed = text.trim_end_matches(['\r', '\n']);
        if let Err(e) = writeln!(state.0, "{prefix} {trimmed}") {
            tracing::warn!("daemon log {}: {e}", self.path.display());
        }
    }

    fn rotate(&self, state: &mut (File, u32)) {
        if rotate_if_needed(&self.path, self.max_bytes).is_err() {
            return;
        }
        if let Ok(file) = open_append(&self.path) {
            state.0 = file;
        }
    }

    fn lock(&self) -> MutexGuard<'_, (File, u32)> {
        self.state.lock().unwrap_or_else(PoisonError::into_inner)
    }
}

fn open_append(path: &Path) -> std::io::Result<File> {
    OpenOptions::new().create(true).append(true).open(path)
}

fn rotate_if_needed(path: &Path, max_len: u64) -> std::io::Result<()> {
    let len = match std::fs::metadata(path) {
        Ok(meta) => meta.len(),
        Err(e) if e.kind() == std::io::ErrorKind::NotFound => return Ok(()),
        Err(e) => return Err(e),
    };
    if len < max_len {
        return Ok(());
    }
    let mut rotated = path.as_os_str().to_owned();
    rotated.push(".1");
    std::fs::rename(path, PathBuf::from(rotated))
}

#[cfg(test)]
#[path = "daemon_log_tests.rs"]
mod tests;
