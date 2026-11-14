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
    pub(crate) fn parse(path: &Path, bytes: &[u8]) -> Result<BTreeMap<HostId, Endpoint>, Error> {
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

    pub(crate) fn save(path: &Path, hosts: &BTreeMap<HostId, Endpoint>) -> Result<(), Error> {
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

fn write_atomically(path: &Path, bytes: &[u8]) -> std::io::Result<()> {
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

fn stage_and_rename(staged: &Path, path: &Path, bytes: &[u8]) -> std::io::Result<()> {
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

fn admit(hosts: &mut BTreeMap<HostId, Endpoint>, entry: HostEntry) -> Result<(), String> {
    check_registrable(&entry.id).map_err(|e| format!("host {}: {e}", entry.id))?;
    if hosts.insert(entry.id.clone(), entry.endpoint).is_some() {
        return Err(format!("host {} is listed twice", entry.id));
    }
    Ok(())
}

#[cfg(test)]
#[path = "hosts_document_tests.rs"]
mod tests;
