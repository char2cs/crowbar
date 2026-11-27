mod catalog;
mod error;
mod pane_host;
mod scheme;
mod slot;
#[cfg(test)]
mod test_support;

pub use catalog::{
    BundleAsset, BundleSource, DirBundle, ExtensionBundles, Multiplicity, PaneBundles, PaneKind,
    PaneLifecycle, PaneRegistry, PaneType,
};
pub use error::{Error, Result};
pub use pane_host::{
    BrowseAction, Covered, Keyboard, LayoutSnapshot, Overlays, PaneDisplay, PaneHost, PaneKinds,
    SyncReport, VisiblePane,
};
pub use protocol::PaneOrigin;
pub use scheme::{Assignment, MenuAsk, OriginTable, PaneMenus, PaneProtocol, PaneTables};
pub use slot::{Config, HostEvent, IpcMessage};
