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
    pub(crate) fn open(
        path: &Path,
        max_bytes: u64,
    ) -> std::io::Result<Self> {
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

    pub(crate) fn write(
        &mut self,
        text: &str,
    ) -> std::io::Result<()> {
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
mod tests {
    use std::os::unix::fs::PermissionsExt;

    use super::*;

    fn mode(path: &Path) -> u32 {
        std::fs::metadata(path).unwrap().permissions().mode() & 0o777
    }

    #[test]
    fn text_is_appended_and_the_file_is_private_to_the_user() {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("daemon.log");
        let mut writer = LogWriter::open(&path, 1 << 20).unwrap();
        writer.write("one\n").unwrap();
        writer.write("two\n").unwrap();
        assert_eq!(std::fs::read_to_string(&path).unwrap(), "one\ntwo\n");
        assert_eq!(mode(&path), 0o600);
    }

    #[test]
    fn an_existing_log_with_wider_permissions_is_narrowed() {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("daemon.log");
        std::fs::write(&path, b"old\n").unwrap();
        std::fs::set_permissions(&path, std::fs::Permissions::from_mode(0o644)).unwrap();
        LogWriter::open(&path, 1 << 20).unwrap();
        assert_eq!(mode(&path), 0o600);
    }

    #[test]
    fn a_full_log_is_rotated_aside_when_it_is_opened() {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("daemon.log");
        std::fs::write(&path, b"0123456789ABCDEF").unwrap();
        LogWriter::open(&path, 10)
            .unwrap()
            .write("fresh\n")
            .unwrap();
        assert_eq!(
            std::fs::read(dir.path().join("daemon.log.1")).unwrap(),
            b"0123456789ABCDEF"
        );
        assert_eq!(std::fs::read_to_string(&path).unwrap(), "fresh\n");
    }

    #[test]
    fn a_log_below_the_cap_is_appended_to_in_place() {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("daemon.log");
        std::fs::write(&path, b"short\n").unwrap();
        LogWriter::open(&path, 100)
            .unwrap()
            .write("more\n")
            .unwrap();
        assert!(!dir.path().join("daemon.log.1").exists());
        assert_eq!(std::fs::read_to_string(&path).unwrap(), "short\nmore\n");
    }

    #[test]
    fn the_log_rotates_by_size_while_it_runs_and_never_passes_the_cap() {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("daemon.log");
        let mut writer = LogWriter::open(&path, 20).unwrap();
        for line in ["aaaaaaaaa\n", "bbbbbbbbb\n", "ccccccccc\n"] {
            writer.write(line).unwrap();
        }
        assert_eq!(
            std::fs::read_to_string(dir.path().join("daemon.log.1")).unwrap(),
            "aaaaaaaaa\nbbbbbbbbb\n"
        );
        assert_eq!(std::fs::read_to_string(&path).unwrap(), "ccccccccc\n");
    }

    #[test]
    fn one_entry_larger_than_the_cap_is_still_written() {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("daemon.log");
        let mut writer = LogWriter::open(&path, 5).unwrap();
        writer.write("a very long entry\n").unwrap();
        assert_eq!(
            std::fs::read_to_string(&path).unwrap(),
            "a very long entry\n"
        );
    }
}
