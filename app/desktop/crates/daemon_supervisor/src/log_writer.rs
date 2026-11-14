use std::fs::{File, OpenOptions};
use std::io::Write;
use std::os::unix::fs::{OpenOptionsExt, PermissionsExt};
use std::path::{Path, PathBuf};

const LOG_MODE: u32 = 0o600;

pub(crate) struct LogWriter {
    path: PathBuf,
    max_bytes: u64,
    file: File,
    written: u64,
}

fn open_append(path: &Path) -> std::io::Result<File> {
    let file = OpenOptions::new()
        .create(true)
        .append(true)
        .mode(LOG_MODE)
        .open(path)?;
    let mode = file.metadata()?.permissions().mode() & 0o777;
    if mode != LOG_MODE {
        file.set_permissions(std::fs::Permissions::from_mode(LOG_MODE))?;
    }
    Ok(file)
}

fn rotated_name(path: &Path) -> PathBuf {
    let mut name = path.as_os_str().to_owned();
    name.push(".1");
    PathBuf::from(name)
}

fn rotate(path: &Path) -> std::io::Result<()> {
    match std::fs::rename(path, rotated_name(path)) {
        Err(e) if e.kind() != std::io::ErrorKind::NotFound => Err(e),
        _ => Ok(()),
    }
}

impl LogWriter {
    pub(crate) fn open(path: &Path, max_bytes: u64) -> std::io::Result<Self> {
        let len = match std::fs::metadata(path) {
            Ok(meta) => meta.len(),
            Err(e) if e.kind() == std::io::ErrorKind::NotFound => 0,
            Err(e) => return Err(e),
        };
        if len >= max_bytes {
            rotate(path)?;
        }
        let file = open_append(path)?;
        let written = file.metadata()?.len();
        Ok(Self {
            path: path.to_path_buf(),
            max_bytes,
            file,
            written,
        })
    }

    pub(crate) fn write(&mut self, text: &str) -> std::io::Result<()> {
        let len = u64::try_from(text.len()).unwrap_or(u64::MAX);
        if self.written > 0 && self.written.saturating_add(len) > self.max_bytes {
            rotate(&self.path)?;
            self.file = open_append(&self.path)?;
            self.written = 0;
        }
        self.file.write_all(text.as_bytes())?;
        self.written = self.written.saturating_add(len);
        Ok(())
    }
}

#[cfg(test)]
#[path = "log_writer_tests.rs"]
mod tests;
