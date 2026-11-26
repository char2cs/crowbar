use std::collections::BTreeMap;
use std::path::Path;

use protocol::HostId;
use serde::{Deserialize, Serialize};

use crate::error::Error;
use crate::hosts::endpoint::Endpoint;
use crate::hosts::entry::Entry;
use crate::hosts::storage::Storage;
use crate::hosts::target::check_registrable;

const FILE_VERSION: u32 = 1;

#[derive(Debug, Serialize, Deserialize)]
pub(crate) struct Document {
    version: u32,
    hosts: Vec<Entry>,
}

fn unsupported_version(
    path: &Path,
    found: u32,
) -> Error {
    Error::HostsVersion {
        path: path.to_path_buf(),
        found,
        supported: FILE_VERSION,
    }
}

fn entries(hosts: &BTreeMap<HostId, Endpoint>) -> Vec<Entry> {
    let entry = |(id, endpoint): (&HostId, &Endpoint)| Entry {
        id: id.clone(),
        endpoint: endpoint.clone(),
    };
    hosts.iter().map(entry).collect()
}

impl Document {
    pub(crate) fn parse(
        path: &Path,
        bytes: &[u8],
    ) -> Result<BTreeMap<HostId, Endpoint>, Error> {
        let corrupt = |source| Error::HostsCorrupt {
            path: path.to_path_buf(),
            source,
        };
        let doc: Self = serde_json::from_slice(bytes).map_err(corrupt)?;
        if doc.version != FILE_VERSION {
            return Err(unsupported_version(path, doc.version));
        }
        doc.hosts
            .into_iter()
            .try_fold(BTreeMap::new(), |mut hosts, entry| {
                insert_registrable_host(path, &mut hosts, entry).map(|()| hosts)
            })
    }

    pub(crate) fn save(
        storage: &dyn Storage,
        hosts: &BTreeMap<HostId, Endpoint>,
    ) -> Result<(), Error> {
        let io = |source| Error::HostsIo {
            path: storage.location().to_path_buf(),
            source,
        };
        let doc = Self {
            version: FILE_VERSION,
            hosts: entries(hosts),
        };
        let bytes = serde_json::to_vec_pretty(&doc).map_err(|e| io(e.into()))?;
        storage.store(&bytes).map_err(io)
    }

    pub(crate) fn load(storage: &dyn Storage) -> Result<BTreeMap<HostId, Endpoint>, Error> {
        let stored = storage.load().map_err(|source| Error::HostsIo {
            path: storage.location().to_path_buf(),
            source,
        })?;
        match stored {
            Some(bytes) => Self::parse(storage.location(), &bytes),
            None => Ok(BTreeMap::new()),
        }
    }
}

fn insert_registrable_host(
    path: &Path,
    hosts: &mut BTreeMap<HostId, Endpoint>,
    entry: Entry,
) -> Result<(), Error> {
    let invalid = |why: String| Error::HostsInvalid {
        path: path.to_path_buf(),
        why,
    };
    check_registrable(&entry.id).map_err(|e| invalid(format!("host {}: {e}", entry.id)))?;
    if hosts.insert(entry.id.clone(), entry.endpoint).is_some() {
        return Err(invalid(format!("host {} is listed twice", entry.id)));
    }
    Ok(())
}

#[cfg(test)]
mod tests {
    use std::path::PathBuf;

    use super::*;

    const CASES: [(&str, &str); 6] = [
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
    ];

    #[test]
    fn a_corrupt_newer_or_unregistrable_file_is_an_error() {
        let path = PathBuf::from("hosts.json");
        for (text, wanted) in CASES {
            let err = Document::parse(&path, text.as_bytes()).unwrap_err();
            assert!(err.to_string().contains(wanted), "{text}: {err}");
        }
    }
}
