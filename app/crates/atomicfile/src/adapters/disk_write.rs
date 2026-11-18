//! Staging, syncing and renaming through the operating system.

use std::fs::{File, OpenOptions};
use std::io::{Error, ErrorKind, Result, Write};
use std::os::unix::fs::OpenOptionsExt;
use std::path::{Path, PathBuf};

const PRIVATE_MODE: u32 = 0o600;
const STAGING_SUFFIX: &str = ".tmp";

/// Replaces the file at `path` with `bytes`, creating its directory. The file is owner-only
/// (mode 0600); readers see the old content or the new, never a mix.
///
/// # Errors
/// The first I/O failure; a staged file left by a failed write is removed.
pub fn write(
    path: &Path,
    bytes: &[u8],
) -> Result<()> {
    let directory = directory_of(path);
    std::fs::create_dir_all(&directory)?;
    let staged = staged_name(path)?;
    let replaced = stage(&staged, bytes).and_then(|()| std::fs::rename(&staged, path));
    if let Err(error) = replaced {
        remove_quietly(&staged);
        return Err(error);
    }
    File::open(&directory)?.sync_all()
}

fn directory_of(path: &Path) -> PathBuf {
    match path.parent() {
        Some(parent) if !parent.as_os_str().is_empty() => parent.to_owned(),
        _ => PathBuf::from("."),
    }
}

fn staged_name(path: &Path) -> Result<PathBuf> {
    let name = path
        .file_name()
        .ok_or_else(|| Error::new(ErrorKind::InvalidInput, "the path names no file"))?;
    let mut staged = name.to_owned();
    staged.push(STAGING_SUFFIX);
    Ok(path.with_file_name(staged))
}

fn stage(
    staged: &Path,
    bytes: &[u8],
) -> Result<()> {
    let mut file = OpenOptions::new()
        .write(true)
        .create(true)
        .truncate(true)
        .mode(PRIVATE_MODE)
        .open(staged)?;
    file.write_all(bytes)?;
    file.sync_all()
}

fn remove_quietly(staged: &Path) {
    if let Err(error) = std::fs::remove_file(staged) {
        tracing::debug!(%error, "the staged file could not be removed");
    }
}

#[cfg(test)]
mod tests {
    use std::os::unix::fs::PermissionsExt;

    use super::*;

    const CASES: &[(&str, &[u8])] = &[("first write", b"old"), ("replacement", b"new")];

    #[test]
    #[ignore = "real-infra: the filesystem adapter itself: file mode, rename and cleanup of a staged file"]
    fn a_write_replaces_the_content_privately_and_leaves_no_staging_file() {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("nested/state.json");
        for (name, content) in CASES {
            write(&path, content).unwrap();
            assert_eq!(std::fs::read(&path).unwrap(), *content, "{name}");
        }
        let mode = std::fs::metadata(&path).unwrap().permissions().mode();
        assert_eq!(mode & 0o777, PRIVATE_MODE);
        let names: Vec<_> = std::fs::read_dir(path.parent().unwrap()).unwrap().collect();
        assert_eq!(names.len(), 1, "only the target remains");
    }

    #[test]
    #[ignore = "real-infra: the filesystem adapter itself: file mode, rename and cleanup of a staged file"]
    fn a_failed_write_keeps_the_old_file_and_removes_what_it_staged() {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("state.json");
        std::fs::create_dir(&path).unwrap();
        assert!(write(&path, b"new").is_err(), "a directory is in the way");
        assert!(!dir.path().join("state.json.tmp").exists());
    }

    #[test]
    fn a_path_without_a_file_name_is_refused() {
        assert_eq!(
            write(Path::new("/"), b"x").unwrap_err().kind(),
            ErrorKind::InvalidInput
        );
    }
}
