//! Pane host: everything between the layout model and the web views.
//!
//! * [`PaneRegistry`] lists the pane types (id, entry page, [`PaneLifecycle`],
//!   [`Multiplicity`]); built-in and extension panes are the same thing to it.
//! * [`PaneHost`] follows the layout: web views live in a pool of slots that follow the
//!   visible panes, hidden panes are parked or kept alive by their type's lifecycle, focus
//!   decides `activate` and `deactivate`, and native overlays hide web views without telling
//!   any pane. A pane whose type is gone shows a placeholder and keeps its state; a pane that
//!   fails shows the error in its own frame.
//! * [`PaneProtocol`] serves the `crowbar://` scheme: built pane bundles from a
//!   [`BundleSource`] and everything else through [`protocol::DaemonTransport`], authorized
//!   per web view so an extension pane reaches only its own bundle and routes.
//!
//! Pages talk to the host with the `sdk@1` frames of the `protocol` crate over ipc. The sender
//! is the pane that owns the web view (a [`PaneHost`] decision), never a pane-supplied id.

mod bootstrap;
mod bundle_asset;
mod bundle_source;
mod dir_bundle;
mod entry_url;
mod error;
mod extension_bundles;
mod host_event;
mod ipc_message;
mod layout_snapshot;
mod mime;
mod multiplicity;
mod origin;
mod origin_table;
mod overlay;
mod pane_display;
mod pane_host;
mod pane_host_config;
mod pane_lifecycle;
mod pane_phase;
mod pane_protocol;
mod pane_registry;
mod pane_runtime;
mod pane_type;
mod policy;
mod route;
mod slot;
mod slot_factory;
mod slot_id;
mod slot_owner;
mod slot_pool;
mod visible_pane;

pub use bundle_asset::BundleAsset;
pub use bundle_source::BundleSource;
pub use dir_bundle::DirBundle;
pub use error::{Error, Result};
pub use extension_bundles::ExtensionBundles;
pub use host_event::HostEvent;
pub use ipc_message::IpcMessage;
pub use layout_snapshot::LayoutSnapshot;
pub use multiplicity::Multiplicity;
pub use origin::Origin;
pub use origin_table::OriginTable;
pub use overlay::Overlay;
pub use pane_display::PaneDisplay;
pub use pane_host::PaneHost;
pub use pane_host_config::PaneHostConfig;
pub use pane_lifecycle::PaneLifecycle;
pub use pane_protocol::PaneProtocol;
pub use pane_registry::PaneRegistry;
pub use pane_type::PaneType;
pub use visible_pane::VisiblePane;
