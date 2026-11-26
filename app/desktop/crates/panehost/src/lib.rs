//! Pane host: everything between the layout model and the web views.
//!
//! * [`PaneRegistry`] lists the pane types (id, entry page, [`PaneLifecycle`],
//!   [`Multiplicity`]); built-in and extension panes are the same thing to it.
//! * [`PaneHost`] follows the layout: web views live in a pool of slots that follow the
//!   visible panes, hidden panes are parked or kept alive by their type's lifecycle, and focus
//!   decides `activate` and `deactivate`. A pane whose type is gone shows a placeholder and
//!   keeps its state; a pane that fails shows the error in its own frame.
//! * [`PaneProtocol`] serves the `crowbar://` scheme: built pane bundles from a
//!   [`BundleSource`] and everything else through [`protocol::DaemonTransport`], authorized
//!   per web view so an extension pane reaches only its own bundle and routes.
//!
//! Pages talk to the host with the `sdk@1` frames of the `protocol` crate over ipc. The sender
//! is the pane that owns the web view (a [`PaneHost`] decision), never a pane-supplied id.

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
