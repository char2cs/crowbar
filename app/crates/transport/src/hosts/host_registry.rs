use std::collections::BTreeMap;
use std::path::PathBuf;
use std::sync::{Arc, Mutex, PoisonError, RwLock, RwLockReadGuard, RwLockWriteGuard};

use protocol::HostId;

use crate::blocking::run_blocking;
use crate::error::Error;
use crate::hosts::adapters::FileStorage;
use crate::hosts::document::Document;
use crate::hosts::endpoint::Endpoint;
use crate::hosts::storage::Storage;
use crate::hosts::target::check_registrable;

type Hosts = BTreeMap<HostId, Endpoint>;

/// Host id to the `host:port` its daemon listens on, optionally backed by a JSON file. Secrets
/// are not here: tokens and pins live in a [`TokenStore`](crate::TokenStore). Changes are
/// written through on the blocking pool; a failed write leaves the list as it was, and readers
/// never wait for the disk.
#[derive(Debug, Default)]
pub struct HostRegistry {
    hosts: RwLock<Hosts>,
    writer: Mutex<()>,
    storage: Option<Arc<dyn Storage>>,
}

impl HostRegistry {
    /// A registry that forgets everything on exit.
    #[must_use]
    pub fn in_memory() -> Self {
        Self::default()
    }

    /// A registry persisted at `path`. A missing file is an empty list; a corrupt one is an
    /// error rather than silently dropping the user's hosts. Reads the file on the calling
    /// thread, so call it at startup or from the blocking pool.
    ///
    /// # Errors
    /// [`Error::HostsIo`], [`Error::HostsCorrupt`], [`Error::HostsInvalid`] or
    /// [`Error::HostsVersion`] when the file cannot be read or understood.
    pub fn open(path: impl Into<PathBuf>) -> Result<Self, Error> {
        Self::with_storage(Arc::new(FileStorage::new(path.into())))
    }

    pub(crate) fn with_storage(storage: Arc<dyn Storage>) -> Result<Self, Error> {
        let hosts = Document::load(storage.as_ref())?;
        Ok(Self {
            hosts: RwLock::new(hosts),
            writer: Mutex::new(()),
            storage: Some(storage),
        })
    }

    /// Where `host_id` listens, `None` when it is not registered.
    #[must_use]
    pub fn get(
        &self,
        host_id: &HostId,
    ) -> Option<Endpoint> {
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
    pub async fn insert(
        self: &Arc<Self>,
        host_id: HostId,
        endpoint: Endpoint,
    ) -> Result<(), Error> {
        let registry = Arc::clone(self);
        run_blocking(move || registry.insert_blocking(host_id, endpoint)).await
    }

    /// Removes `host_id`; removing an unknown host is not an error.
    ///
    /// # Errors
    /// A hosts-file error when the list cannot be saved.
    pub async fn remove(
        self: &Arc<Self>,
        host_id: HostId,
    ) -> Result<(), Error> {
        let registry = Arc::clone(self);
        run_blocking(move || registry.remove_blocking(&host_id)).await
    }

    pub(crate) fn insert_blocking(
        &self,
        host_id: HostId,
        endpoint: Endpoint,
    ) -> Result<(), Error> {
        check_registrable(&host_id)?;
        self.change(|hosts| {
            hosts.insert(host_id, endpoint);
        })
    }

    pub(crate) fn remove_blocking(
        &self,
        host_id: &HostId,
    ) -> Result<(), Error> {
        self.change(|hosts| {
            hosts.remove(host_id);
        })
    }

    fn change(
        &self,
        edit: impl FnOnce(&mut Hosts),
    ) -> Result<(), Error> {
        let _writer = self.writer.lock().unwrap_or_else(PoisonError::into_inner);
        let mut next = self.read().clone();
        edit(&mut next);
        if let Some(storage) = &self.storage {
            Document::save(storage.as_ref(), &next)?;
        }
        *self.write() = next;
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
mod tests {
    use super::*;
    use crate::hosts::test_support::MemoryStorage;

    fn id(text: &str) -> HostId {
        HostId::new(text).unwrap()
    }

    fn at(text: &str) -> Endpoint {
        Endpoint::parse(text).unwrap()
    }

    #[test]
    fn insert_get_remove_round_trip() {
        let registry = HostRegistry::in_memory();
        registry
            .insert_blocking(id("host-1"), at("1.2.3.4:3737"))
            .unwrap();
        assert_eq!(registry.get(&id("host-1")), Some(at("1.2.3.4:3737")));
        registry.remove_blocking(&id("host-1")).unwrap();
        assert_eq!(registry.get(&id("host-1")), None);
        assert_eq!(registry.list().len(), 0);
    }

    const CASES: [&str; 2] = ["local", "localhost"];

    #[test]
    fn ids_the_router_would_send_to_the_local_daemon_are_refused() {
        let registry = HostRegistry::in_memory();
        for text in CASES {
            assert!(
                registry.insert_blocking(id(text), at("1.2.3.4:1")).is_err(),
                "{text}"
            );
        }
        assert_eq!(registry.list().len(), 0);
    }

    fn reopened(storage: &Arc<MemoryStorage>) -> HostRegistry {
        let shared: Arc<dyn Storage> = Arc::<MemoryStorage>::clone(storage);
        HostRegistry::with_storage(shared).unwrap()
    }

    #[test]
    fn hosts_survive_a_restart() {
        let storage = MemoryStorage::shared();
        let first = reopened(&storage);
        first
            .insert_blocking(id("host-2"), at("[::1]:4000"))
            .unwrap();
        first
            .insert_blocking(id("host-1"), at("a.example:3737"))
            .unwrap();
        first.remove_blocking(&id("host-2")).unwrap();

        let second = reopened(&storage);

        assert_eq!(second.list(), vec![(id("host-1"), at("a.example:3737"))]);
    }

    #[test]
    fn nothing_stored_is_an_empty_list() {
        assert_eq!(reopened(&MemoryStorage::shared()).list().len(), 0);
    }

    #[test]
    fn a_failed_write_leaves_the_list_as_it_was() {
        let storage = MemoryStorage::shared();
        let registry = reopened(&storage);
        storage.fail_writes();

        let result = registry.insert_blocking(id("host-1"), at("1.2.3.4:1"));

        assert!(matches!(result, Err(Error::HostsIo { .. })));
        assert_eq!(registry.list().len(), 0);
    }

    #[tokio::test]
    async fn the_async_writers_persist_through_the_blocking_pool() {
        let storage = MemoryStorage::shared();
        let registry = Arc::new(reopened(&storage));
        registry
            .insert(id("host-1"), at("1.2.3.4:1"))
            .await
            .unwrap();
        assert_eq!(reopened(&storage).list().len(), 1);
        registry.remove(id("host-1")).await.unwrap();
        assert_eq!(reopened(&storage).list().len(), 0);
    }

    #[test]
    #[ignore = "real-infra: asserts the 0600 and 0700 modes of a real file and directory"]
    fn a_file_registry_keeps_its_list_in_a_private_file_and_directory() {
        use std::os::unix::fs::PermissionsExt;
        use std::path::Path;

        let dir = tempfile::TempDir::new().unwrap();
        let path = dir.path().join("sub").join("hosts.json");
        HostRegistry::open(&path)
            .unwrap()
            .insert_blocking(id("host-1"), at("a.example:3737"))
            .unwrap();
        let mode = |p: &Path| std::fs::metadata(p).unwrap().permissions().mode() & 0o777;
        assert_eq!(mode(&path), 0o600, "file");
        assert_eq!(mode(path.parent().unwrap()), 0o700, "directory");
        assert_eq!(HostRegistry::open(&path).unwrap().list().len(), 1);
    }

    #[test]
    fn readers_are_not_blocked_while_a_write_is_on_disk() {
        let registry = reopened(&MemoryStorage::shared());
        registry
            .insert_blocking(id("host-1"), at("1.2.3.4:1"))
            .unwrap();
        let writing = registry.writer.lock().unwrap();
        assert_eq!(registry.list().len(), 1, "reads need only the read lock");
        drop(writing);
    }
}
