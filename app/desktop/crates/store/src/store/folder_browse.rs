//! The remote folder picker's data: a host's own directories, listed on demand.

mod browse_entry;
mod browse_wire;
pub(crate) mod folder_choice;
mod folder_listing;
mod followed;
mod held;

pub use browse_entry::BrowseEntry;
pub use folder_choice::FolderChoice;
pub use folder_listing::FolderListing;

use std::sync::{Arc, Mutex};

use sidebar::HostState;
use tokio::runtime::Handle;

use crate::error::Error;
use crate::live::ChangeSignal;
use crate::live::Hub;
use crate::store::folder_browse::browse_wire::BrowseWire;
use crate::store::folder_browse::followed::Followed;
use crate::store::folder_browse::held::Held;

/// The directory the picker starts from.
pub const ROOT_PATH: &str = "/";

/// Lists directories of the host that holds the open space, as the picker opens them.
pub struct FolderBrowse {
    host: Followed,
    hub: Arc<Hub>,
    handle: Handle,
    signal: ChangeSignal,
    held: Mutex<Held>,
}

impl std::fmt::Debug for FolderBrowse {
    fn fmt(
        &self,
        formatter: &mut std::fmt::Formatter<'_>,
    ) -> std::fmt::Result {
        formatter
            .debug_struct("FolderBrowse")
            .field("host", &self.host.id)
            .finish_non_exhaustive()
    }
}

impl FolderBrowse {
    pub(crate) fn open(
        (hub, handle): (&Arc<Hub>, &Handle),
        signal: ChangeSignal,
    ) -> Result<Arc<Self>, Error> {
        let host = hub
            .read(followed::of_open_space)
            .ok_or_else(|| Error::Stream("no space is open".to_owned()))?;
        Ok(Self::on(host, (hub, handle), signal))
    }

    pub(crate) fn open_on(
        (hub, handle): (&Arc<Hub>, &Handle),
        host: &str,
        signal: ChangeSignal,
    ) -> Result<Arc<Self>, Error> {
        let found = hub
            .read(|state| followed::named(state, host))
            .ok_or_else(|| Error::Stream(format!("host {host} is not followed")))?;
        Ok(Self::on(found, (hub, handle), signal))
    }

    fn on(
        host: Followed,
        (hub, handle): (&Arc<Hub>, &Handle),
        signal: ChangeSignal,
    ) -> Arc<Self> {
        Arc::new(Self {
            host,
            hub: Arc::clone(hub),
            handle: handle.clone(),
            signal,
            held: Mutex::new(Held::default()),
        })
    }

    /// The name the host is shown under.
    pub fn host_name(&self) -> String {
        self.host.name.clone()
    }

    pub(crate) fn reach(&self) -> HostState {
        let id = &self.host.id;
        self.hub.read(|state| {
            state
                .hosts
                .iter()
                .find(|slot| slot.spec.id == *id)
                .map_or(HostState::Unknown, |slot| slot.link.host_state())
        })
    }

    /// Whether the host can be asked anything now.
    pub fn can_ask(&self) -> bool {
        self.reach() != HostState::Down
    }

    /// What `path` shows as far as the host has answered; `None` until it was asked for.
    pub fn listing(
        &self,
        path: &str,
    ) -> Option<FolderListing> {
        self.held.lock().ok()?.listing(path)
    }

    /// Asks the host for `path` once, and only while it can be asked.
    pub fn request(
        self: &Arc<Self>,
        path: &str,
    ) {
        let unasked = self.held.lock().is_ok_and(|held| !held.is_listed(path));
        if unasked && self.can_ask() {
            self.fetch(path);
        }
    }

    /// Asks for `path` again after it failed.
    pub fn retry(
        self: &Arc<Self>,
        path: &str,
    ) {
        let failed = self.held.lock().is_ok_and(|held| held.is_failed(path));
        if failed && self.can_ask() {
            self.fetch(path);
        }
    }

    fn fetch(
        self: &Arc<Self>,
        path: &str,
    ) {
        let Some(request) = self.held.lock().ok().map(|mut held| held.begin(path)) else {
            return;
        };
        let (me, path) = (Arc::clone(self), path.to_owned());
        self.handle
            .spawn(async move { me.load(&path, request).await });
    }

    async fn load(
        &self,
        path: &str,
        request: u64,
    ) {
        let route = daemonapi::Route::browse(path);
        let listing = self
            .host
            .client
            .read::<Vec<BrowseWire>>(route)
            .await
            .inspect_err(|error| tracing::warn!(path, %error, "a folder could not be listed"))
            .map_or_else(
                |error| FolderListing::Failed(error.to_string()),
                |rows| {
                    FolderListing::Loaded(rows.into_iter().map(BrowseWire::into_entry).collect())
                },
            );
        let settled = self
            .held
            .lock()
            .is_ok_and(|mut held| held.settle(path, request, listing));
        if settled {
            (self.signal)();
        }
    }
}

#[cfg(test)]
mod tests {
    use tokio::sync::Notify;

    use super::*;
    use daemonapi::EntryKind;

    use crate::live::LinkState;
    use crate::store::folder_browse::browse_entry::BrowseEntry;
    use crate::test_support::{FakeDaemon, state};

    const ROOT: &str = r#"[{"name":"srv","path":"/srv","isDir":true},{"name":"a.txt","path":"/a.txt","isDir":false}]"#;
    const REFUSAL: &str = r#"{"success":false,"error":"cannot read path"}"#;

    type Opened = (Arc<FolderBrowse>, Arc<FakeDaemon>, Arc<Notify>);

    fn hush() {}

    fn quiet() -> ChangeSignal {
        Arc::new(hush)
    }

    fn open_with(setup: impl FnOnce(&FakeDaemon)) -> Opened {
        let (state, daemon) = state();
        setup(&daemon);
        let hub = Hub::new(state, Arc::new(hush));
        let changed = Arc::new(Notify::new());
        let wake = Arc::clone(&changed);
        let signal: ChangeSignal = Arc::new(move || wake.notify_one());
        let browse = FolderBrowse::open((&hub, &Handle::current()), signal).unwrap();
        (browse, daemon, changed)
    }

    async fn settled(
        browse: &FolderBrowse,
        changed: &Notify,
        path: &str,
    ) -> FolderListing {
        while matches!(browse.listing(path), None | Some(FolderListing::Loading)) {
            changed.notified().await;
        }
        browse.listing(path).unwrap_or(FolderListing::Loading)
    }

    fn loaded(listing: FolderListing) -> Vec<BrowseEntry> {
        match listing {
            FolderListing::Loaded(rows) => rows,
            other => panic!("the root was listed, got {other:?}"),
        }
    }

    fn setup_routes(daemon: &FakeDaemon) {
        daemon.ok("GET", "/v0/system/browse?path=/", ROOT);
        daemon.answer(("GET", "/v0/system/browse?path=/no%20way"), 400, REFUSAL);
    }

    #[tokio::test]
    async fn a_directory_is_listed_once_a_refusal_shows_its_reason_and_retry_asks_again() {
        let (browse, daemon, changed) = open_with(setup_routes);
        let parts = (&browse.hub, &browse.handle);
        assert!(FolderBrowse::open_on(parts, "local", quiet()).is_ok());
        assert!(FolderBrowse::open_on(parts, "nowhere", quiet()).is_err());
        assert_eq!(browse.listing(ROOT_PATH), None);
        browse.request(ROOT_PATH);
        browse.request(ROOT_PATH);
        let rows = loaded(settled(&browse, &changed, ROOT_PATH).await);
        let seen: Vec<_> = rows
            .iter()
            .map(|row| (row.name.as_str(), row.kind))
            .collect();
        assert_eq!(
            seen,
            [("srv", EntryKind::Folder), ("a.txt", EntryKind::File)]
        );
        browse.request("/no way");
        let failed = settled(&browse, &changed, "/no way").await;
        assert_eq!(failed, FolderListing::Failed("cannot read path".to_owned()));
        browse.retry("/no way");
        assert_eq!(browse.listing("/no way"), Some(FolderListing::Loading));
        settled(&browse, &changed, "/no way").await;
        let asked = daemon
            .seen()
            .iter()
            .filter(|call| call.path.contains("no%20way"))
            .count();
        assert_eq!(asked, 2);
        assert_eq!(
            daemon
                .seen()
                .iter()
                .filter(|call| call.path.ends_with("path=/"))
                .count(),
            1
        );
        assert_eq!(browse.host_name(), "This computer");
    }

    #[tokio::test]
    async fn a_host_that_is_down_is_not_asked() {
        let (state, daemon) = state();
        let hub = Hub::new(state, Arc::new(hush));
        hub.edit(|state| state.hosts[0].link = LinkState::Down);
        let browse = FolderBrowse::open((&hub, &Handle::current()), Arc::new(hush)).unwrap();
        browse.request(ROOT_PATH);
        assert_eq!(browse.listing(ROOT_PATH), None);
        assert!(!browse.can_ask());
        assert_eq!(browse.reach(), HostState::Down);
        assert_eq!(daemon.seen().len(), 0);
    }
}
