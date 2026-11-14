use std::collections::BTreeMap;
use std::path::PathBuf;
use std::sync::{PoisonError, RwLock, RwLockReadGuard, RwLockWriteGuard};

use protocol::HostId;

use crate::endpoint::Endpoint;
use crate::error::Error;
use crate::host_target::check_registrable;
use crate::hosts_document::HostsDocument;

type Hosts = BTreeMap<HostId, Endpoint>;

/// Host id to the `host:port` its daemon listens on, optionally backed by a JSON file. Secrets
/// are not here: tokens and pins live in the [`TokenStore`](crate::TokenStore). Changes are
/// written through; a failed write leaves the list as it was.
#[derive(Debug, Default)]
pub struct HostRegistry {
    hosts: RwLock<Hosts>,
    file: Option<PathBuf>,
}

impl HostRegistry {
    /// A registry that forgets everything on exit.
    #[must_use]
    pub fn in_memory() -> Self {
        Self::default()
    }

    /// A registry persisted at `path`. A missing file is an empty list; a corrupt one is an
    /// error rather than silently dropping the user's hosts.
    ///
    /// # Errors
    /// [`Error::HostsIo`], [`Error::HostsCorrupt`] or [`Error::HostsVersion`] when the file
    /// cannot be read or understood.
    pub fn open(path: impl Into<PathBuf>) -> Result<Self, Error> {
        let path = path.into();
        let hosts = match std::fs::read(&path) {
            Ok(bytes) => HostsDocument::parse(&path, &bytes)?,
            Err(e) if e.kind() == std::io::ErrorKind::NotFound => Hosts::new(),
            Err(source) => return Err(Error::HostsIo { path, source }),
        };
        Ok(Self {
            hosts: RwLock::new(hosts),
            file: Some(path),
        })
    }

    /// Where `host_id` listens, `None` when it is not registered.
    #[must_use]
    pub fn get(&self, host_id: &HostId) -> Option<Endpoint> {
        self.read().get(host_id).cloned()
    }

    /// Every registered host, ordered by id.
    #[must_use]
    pub fn list(&self) -> Vec<(HostId, Endpoint)> {
        self.read()
            .iter()
            .map(|(id, at)| (id.clone(), at.clone()))
            .collect()
    }

    /// Registers or moves `host_id`.
    ///
    /// # Errors
    /// [`Error::BadHostId`] for an id that names the local daemon, or a hosts-file error when
    /// the list cannot be saved.
    pub fn insert(&self, host_id: HostId, endpoint: Endpoint) -> Result<(), Error> {
        check_registrable(&host_id)?;
        self.change(|hosts| {
            hosts.insert(host_id, endpoint);
        })
    }

    /// Removes `host_id`; removing an unknown host is not an error.
    ///
    /// # Errors
    /// A hosts-file error when the list cannot be saved.
    pub fn remove(&self, host_id: &HostId) -> Result<(), Error> {
        self.change(|hosts| {
            hosts.remove(host_id);
        })
    }

    fn change(&self, edit: impl FnOnce(&mut Hosts)) -> Result<(), Error> {
        let mut hosts = self.write();
        let mut next = hosts.clone();
        edit(&mut next);
        if let Some(path) = &self.file {
            HostsDocument::save(path, &next)?;
        }
        *hosts = next;
        Ok(())
    }

    fn read(&self) -> RwLockReadGuard<'_, Hosts> {
        self.hosts.read().unwrap_or_else(PoisonError::into_inner)
    }

    fn write(&self) -> RwLockWriteGuard<'_, Hosts> {
        self.hosts.write().unwrap_or_else(PoisonError::into_inner)
    }
}

#[cfg(test)]
#[path = "host_registry_tests.rs"]
mod tests;
