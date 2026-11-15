use std::collections::BTreeMap;
use std::fs::{DirBuilder, File, OpenOptions};
use std::io::Write;
use std::os::unix::fs::{DirBuilderExt, OpenOptionsExt};
use std::path::{Path, PathBuf};

use protocol::HostId;
use serde::{Deserialize, Serialize};

use crate::endpoint::Endpoint;
use crate::error::Error;
use crate::host_entry::HostEntry;
use crate::host_target::check_registrable;

const FILE_VERSION: u32 = 1;
const PRIVATE_FILE: u32 = 0o600;
const PRIVATE_DIR: u32 = 0o700;

#[derive(Debug, Serialize, Deserialize)]
pub(crate) struct HostsDocument {
    version: u32,
    hosts: Vec<HostEntry>,
}

impl HostsDocument {
    pub(crate) fn parse(
        path: &Path,
        bytes: &[u8],
    ) -> Result<BTreeMap<HostId, Endpoint>, Error> {
        let doc: Self = serde_json::from_slice(bytes).map_err(|source| Error::HostsCorrupt {
            path: path.to_path_buf(),
            source,
        })?;
        if doc.version != FILE_VERSION {
            return Err(Error::HostsVersion {
                path: path.to_path_buf(),
                found: doc.version,
                supported: FILE_VERSION,
            });
        }
        let invalid = |why: String| Error::HostsInvalid {
            path: path.to_path_buf(),
            why,
        };
        let mut hosts = BTreeMap::new();
        for entry in doc.hosts {
            admit(&mut hosts, entry).map_err(invalid)?;
        }
        Ok(hosts)
    }

    pub(crate) fn save(
        path: &Path,
        hosts: &BTreeMap<HostId, Endpoint>,
    ) -> Result<(), Error> {
        let io = |source| Error::HostsIo {
            path: path.to_path_buf(),
            source,
        };
        let doc = Self {
            version: FILE_VERSION,
            hosts: hosts
                .iter()
                .map(|(id, endpoint)| HostEntry {
                    id: id.clone(),
                    endpoint: endpoint.clone(),
                })
                .collect(),
        };
        let bytes = serde_json::to_vec_pretty(&doc).map_err(|e| io(e.into()))?;
        if let Some(dir) = path.parent() {
            DirBuilder::new()
                .recursive(true)
                .mode(PRIVATE_DIR)
                .create(dir)
                .map_err(io)?;
        }
        write_atomically(path, &bytes).map_err(io)
    }
}

fn staged_name(path: &Path) -> PathBuf {
    let mut staged = path.as_os_str().to_owned();
    staged.push(".tmp");
    PathBuf::from(staged)
}

fn write_atomically(
    path: &Path,
    bytes: &[u8],
) -> std::io::Result<()> {
    let staged = staged_name(path);
    let written = stage_and_rename(&staged, path, bytes);
    if written.is_err()
        && let Err(e) = std::fs::remove_file(&staged)
    {
        tracing::debug!("hosts file temp copy not removed: {e}");
    }
    written?;
    match path.parent() {
        Some(dir) => File::open(dir)?.sync_all(),
        None => Ok(()),
    }
}

fn stage_and_rename(
    staged: &Path,
    path: &Path,
    bytes: &[u8],
) -> std::io::Result<()> {
    let mut file = OpenOptions::new()
        .write(true)
        .create(true)
        .truncate(true)
        .mode(PRIVATE_FILE)
        .open(staged)?;
    file.write_all(bytes)?;
    file.sync_all()?;
    std::fs::rename(staged, path)
}

fn admit(
    hosts: &mut BTreeMap<HostId, Endpoint>,
    entry: HostEntry,
) -> Result<(), String> {
    check_registrable(&entry.id).map_err(|e| format!("host {}: {e}", entry.id))?;
    if hosts.insert(entry.id.clone(), entry.endpoint).is_some() {
        return Err(format!("host {} is listed twice", entry.id));
    }
    Ok(())
}

#[cfg(test)]
mod tests {
    use std::os::unix::fs::PermissionsExt;

    use tempfile::TempDir;

    use super::*;

    fn scratch() -> (TempDir, PathBuf) {
        let dir = TempDir::new().unwrap();
        let path = dir.path().join("sub").join("hosts.json");
        (dir, path)
    }

    fn one_host() -> BTreeMap<HostId, Endpoint> {
        let mut hosts = BTreeMap::new();
        hosts.insert(
            HostId::new("host-1").unwrap(),
            Endpoint::parse("a.example:3737").unwrap(),
        );
        hosts
    }

    #[test]
    fn a_saved_list_reads_back_privately_and_leaves_no_temp_file() {
        let (_dir, path) = scratch();
        HostsDocument::save(&path, &one_host()).unwrap();
        let read = HostsDocument::parse(&path, &std::fs::read(&path).unwrap()).unwrap();
        assert_eq!(read, one_host());
        let mode = |p: &Path| std::fs::metadata(p).unwrap().permissions().mode() & 0o777;
        assert_eq!(mode(&path), PRIVATE_FILE, "file");
        assert_eq!(mode(path.parent().unwrap()), PRIVATE_DIR, "directory");
        assert!(!staged_name(&path).exists(), "temp copy renamed away");
    }

    #[test]
    fn a_failed_save_removes_its_temp_copy() {
        let (_dir, path) = scratch();
        HostsDocument::save(&path, &one_host()).unwrap();
        std::fs::remove_file(&path).unwrap();
        std::fs::create_dir(&path).unwrap();
        assert!(
            HostsDocument::save(&path, &one_host()).is_err(),
            "cannot replace a directory"
        );
        assert!(!staged_name(&path).exists(), "no stray temp copy");
    }

    #[test]
    fn a_corrupt_newer_or_unregistrable_file_is_an_error() {
        let path = PathBuf::from("hosts.json");
        for (text, wanted) in [
            ("{not json", "corrupt"),
            (r#"{"version":9,"hosts":[]}"#, "version 9"),
            (
                r#"{"version":1,"hosts":[{"id":"Bad Id","endpoint":"a:1"}]}"#,
                "corrupt",
            ),
            (
                r#"{"version":1,"hosts":[{"id":"ok","endpoint":"nope"}]}"#,
                "corrupt",
            ),
            (
                r#"{"version":1,"hosts":[{"id":"localhost","endpoint":"a:1"}]}"#,
                "host localhost",
            ),
            (
                r#"{"version":1,"hosts":[{"id":"a","endpoint":"a:1"},{"id":"a","endpoint":"b:2"}]}"#,
                "host a is listed twice",
            ),
        ] {
            let err = HostsDocument::parse(&path, text.as_bytes()).unwrap_err();
            assert!(err.to_string().contains(wanted), "{text}: {err}");
        }
    }
}
