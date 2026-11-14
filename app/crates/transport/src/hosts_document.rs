use std::collections::BTreeMap;
use std::io::Write;
use std::path::{Path, PathBuf};

use protocol::HostId;
use serde::{Deserialize, Serialize};

use crate::endpoint::Endpoint;
use crate::error::Error;
use crate::host_entry::HostEntry;
use crate::host_target::check_registrable;

const FILE_VERSION: u32 = 1;

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
        doc.hosts
            .into_iter()
            .map(|entry| check_registrable(&entry.id).map(|()| (entry.id, entry.endpoint)))
            .collect()
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
            std::fs::create_dir_all(dir).map_err(io)?;
        }
        write_atomically(path, &bytes).map_err(io)
    }
}

fn write_atomically(path: &Path, bytes: &[u8]) -> std::io::Result<()> {
    let mut staged = path.as_os_str().to_owned();
    staged.push(".tmp");
    let staged = PathBuf::from(staged);
    let mut file = std::fs::File::create(&staged)?;
    file.write_all(bytes)?;
    file.sync_all()?;
    std::fs::rename(&staged, path)
}

#[cfg(test)]
#[path = "hosts_document_tests.rs"]
mod tests;
